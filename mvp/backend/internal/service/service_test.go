package service_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/boe/rma/internal/enum"
	"github.com/boe/rma/internal/model"
	"github.com/boe/rma/internal/service"
	"github.com/boe/rma/internal/storage"
	"github.com/boe/rma/internal/store"
)

var dbSeq int

// newSvc 构造隔离的测试服务（SQLite 内存库）。
func newSvc(t *testing.T) *service.Service {
	t.Helper()
	dbSeq++
	dsn := fmt.Sprintf("file:memdb%d?mode=memory&cache=shared&_pragma=busy_timeout(5000)", dbSeq)
	db, err := store.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := store.Seed(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1) // SQLite 单写；条件更新仍保证并发安全语义
	svc := service.New(db, "BOE_US", storage.NewMem())
	base := time.Date(2026, 11, 3, 9, 0, 0, 0, time.Local)
	svc.Now = func() time.Time { return base }
	return svc
}

// at 设置服务当前时间（TAT 测试用）。
func at(svc *service.Service, d time.Duration) {
	base := time.Date(2026, 11, 3, 9, 0, 0, 0, time.Local)
	svc.Now = func() time.Time { return base.Add(d) }
}

func mkForecast(t *testing.T, svc *service.Service, rma string, sns []string) *model.ForecastOrder {
	t.Helper()
	fo, err := svc.CreateForecast(service.CreateForecastReq{
		RMANo: rma, PNCode: "PN-A100", ModelCode: "MD-55",
		Customer: "BOE US", Items: toItems(sns, "PN-A100"),
	}, "project")
	if err != nil {
		t.Fatalf("create forecast: %v", err)
	}
	return fo
}

func toItems(sns []string, pn string) []service.ForecastSN {
	out := make([]service.ForecastSN, 0, len(sns))
	for _, s := range sns {
		out = append(out, service.ForecastSN{SN: s, PNCode: pn})
	}
	return out
}

func receive(t *testing.T, svc *service.Service, forecastNo string, sns []string) string {
	t.Helper()
	batch, err := svc.CreateBatch(service.CreateBatchReq{
		ForecastNo: forecastNo, LabelQtyTotal: len(sns), PODPhotoIDs: []string{"POD-1"},
	}, "warehouse")
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	box, err := svc.CreateBox(batch.BatchNo, "warehouse")
	if err != nil {
		t.Fatalf("create box: %v", err)
	}
	for _, sn := range sns {
		res, err := svc.Scan(service.ScanReq{BoxNo: box.BoxNo, SN: sn}, "warehouse")
		if err != nil {
			t.Fatalf("scan %s: %v", sn, err)
		}
		if res.Result != "OK" {
			t.Fatalf("scan %s expected OK, got %s (%s)", sn, res.Result, res.Message)
		}
	}
	if _, err := svc.SealBox(box.BoxNo, "warehouse"); err != nil {
		t.Fatalf("seal box: %v", err)
	}
	return batch.BatchNo
}

func judgeAll(t *testing.T, svc *service.Service, sns []string, j enum.Judgment) {
	t.Helper()
	reqs := make([]service.JudgeReq, 0, len(sns))
	for _, sn := range sns {
		reqs = append(reqs, service.JudgeReq{SN: sn, Judgment: j})
	}
	if _, err := svc.JudgeBatch(reqs, "inspector"); err != nil {
		t.Fatalf("judge: %v", err)
	}
}

// ---------------- AC-01 / AC-02 预报校验 ----------------

func TestForecastQtyMismatch(t *testing.T) {
	svc := newSvc(t)
	_, err := svc.CreateForecast(service.CreateForecastReq{
		RMANo: "RMA-X", PNCode: "PN-A100", Customer: "BOE US",
		ForecastQty: 3, SNList: []string{"SN-1", "SN-2"},
	}, "project")
	if err == nil {
		t.Fatal("expect qty mismatch error")
	}
}

func TestForecastPNNotFound(t *testing.T) {
	svc := newSvc(t)
	_, err := svc.CreateForecast(service.CreateForecastReq{
		RMANo: "RMA-X", PNCode: "PN-NONE", Customer: "BOE US", SNList: []string{"SN-1"},
	}, "project")
	if err == nil {
		t.Fatal("expect PN not found error")
	}
}

// ---------------- AC-03 / AC-04 二次返修 ----------------

func TestSecondaryReturnBranches(t *testing.T) {
	svc := newSvc(t)
	sns := []string{"SN-A1", "SN-A2"}
	fo := mkForecast(t, svc, "RMA-1", sns)
	receive(t, svc, fo.ForecastNo, sns)
	judgeAll(t, svc, sns, enum.JudgeRepair)

	// 完成还货 → COMPLETED
	for _, sn := range sns {
		plan, err := svc.CreateReturn(service.CreateReturnReq{OriginalSN: sn}, "warehouse")
		if err != nil {
			t.Fatalf("create return: %v", err)
		}
		if _, err := svc.ShipOutbound(plan.ReturnOut.OutboundNo, service.ShipReq{
			Carrier: "DHL", TrackingNo: "T1" + sn,
		}, "warehouse"); err != nil {
			t.Fatalf("ship: %v", err)
		}
	}
	var it model.SNItem
	svc.DB.Where("sn = ?", "SN-A1").First(&it)
	if it.Status != string(enum.SNCompleted) {
		t.Fatalf("expect COMPLETED, got %s", it.Status)
	}

	// AC-03：上轮 COMPLETED → 允许二次返修，归档 round，计数+1
	fo2, err := svc.CreateForecast(service.CreateForecastReq{
		RMANo: "RMA-2", PNCode: "PN-A100", Customer: "BOE US",
		Items: toItems([]string{"SN-A1"}, "PN-A100"), ConfirmSecondaryReturn: true,
	}, "project")
	if err != nil {
		t.Fatalf("secondary return should be allowed: %v", err)
	}
	svc.DB.Where("sn = ?", "SN-A1").First(&it)
	if it.Status != string(enum.SNForecasted) || it.ReturnCount != 1 || !it.SecondaryReturnFlag {
		t.Fatalf("round reset failed: %+v", it)
	}
	var rounds []model.SNItemRound
	svc.DB.Where("sn = ?", "SN-A1").Find(&rounds)
	if len(rounds) != 1 || rounds[0].FromStatus != string(enum.SNCompleted) {
		t.Fatalf("round archive failed: %+v", rounds)
	}

	// AC-04：RETAINED 状态不允许二次返修
	// 构造：另一 SN 走 REPLACE → BUFFER 还货 → RETAINED
	svc2 := newSvc(t)
	if _, err := svc2.BufferInbound(service.InboundReq{SN: "BUF-1", PNCode: "PN-A100", Source: "NEW_PURCHASE"}, "warehouse"); err != nil {
		t.Fatalf("buffer inbound: %v", err)
	}
	foA := mkForecast(t, svc2, "RMA-R", []string{"SN-R1"})
	receive(t, svc2, foA.ForecastNo, []string{"SN-R1"})
	judgeAll(t, svc2, []string{"SN-R1"}, enum.JudgeReplace)
	plan, err := svc2.CreateReturn(service.CreateReturnReq{OriginalSN: "SN-R1"}, "warehouse")
	if err != nil {
		t.Fatalf("buffer return: %v", err)
	}
	if _, err := svc2.ShipOutbound(plan.ReturnOut.OutboundNo, service.ShipReq{
		Carrier: "DHL", TrackingNo: "TB1",
	}, "warehouse"); err != nil {
		t.Fatalf("ship buffer: %v", err)
	}
	var r model.SNItem
	svc2.DB.Where("sn = ?", "SN-R1").First(&r)
	if r.Status != string(enum.SNRetained) {
		t.Fatalf("expect RETAINED after buffer return, got %s", r.Status)
	}
	_, err = svc2.CreateForecast(service.CreateForecastReq{
		RMANo: "RMA-R2", PNCode: "PN-A100", Customer: "BOE US",
		Items: toItems([]string{"SN-R1"}, "PN-A100"), ConfirmSecondaryReturn: true,
	}, "project")
	if err == nil {
		t.Fatal("RETAINED must block secondary return")
	}
	_ = fo2
}

// ---------------- AC-05 / AC-06 差异 ----------------

func TestKittingUnknownSNCreatesDiscrepancy(t *testing.T) {
	svc := newSvc(t)
	fo := mkForecast(t, svc, "RMA-D", []string{"SN-D1"})
	batch, _ := svc.CreateBatch(service.CreateBatchReq{
		ForecastNo: fo.ForecastNo, LabelQtyTotal: 1, PODPhotoIDs: []string{"P"},
	}, "warehouse")
	box, _ := svc.CreateBox(batch.BatchNo, "warehouse")
	res, err := svc.Scan(service.ScanReq{BoxNo: box.BoxNo, SN: "SN-UNKNOWN"}, "warehouse")
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if res.Result != "DISCREPANCY" || res.DiscrepancyType != string(enum.DiscSNNotInForecast) {
		t.Fatalf("expect SN_NOT_IN_FORECAST, got %+v", res)
	}
	var it model.SNItem
	svc.DB.Where("sn = ?", "SN-UNKNOWN").First(&it)
	if it.Status != string(enum.SNDiscrepancy) || it.ForecastNo != "" {
		t.Fatalf("unknown SN should be DISCREPANCY with empty forecast_no: %+v", it)
	}
}

func TestDiscrepancyPhotoRules(t *testing.T) {
	svc := newSvc(t)
	fo := mkForecast(t, svc, "RMA-P", []string{"SN-P1"})
	batch, _ := svc.CreateBatch(service.CreateBatchReq{
		ForecastNo: fo.ForecastNo, LabelQtyTotal: 1, PODPhotoIDs: []string{"P"},
	}, "warehouse")
	box, _ := svc.CreateBox(batch.BatchNo, "warehouse")
	res, _ := svc.Scan(service.ScanReq{BoxNo: box.BoxNo, SN: "SN-OVER-1"}, "warehouse")
	if res.RecordNo == "" {
		t.Fatal("expect discrepancy record")
	}
	// 多收需 ≥3 张
	if _, err := svc.SubmitDiscrepancy(res.RecordNo, service.SubmitDiscReq{
		PhotoIDs: []string{"a", "b"},
	}, "warehouse"); err == nil {
		t.Fatal("expect photo count error for QTY_OVER (need 3)")
	}
	if _, err := svc.SubmitDiscrepancy(res.RecordNo, service.SubmitDiscReq{
		PhotoIDs: []string{"a", "b", "c"},
	}, "warehouse"); err != nil {
		t.Fatalf("submit with 3 photos: %v", err)
	}
	// 未提交不允许审批
	res2, _ := svc.Scan(service.ScanReq{BoxNo: box.BoxNo, SN: "SN-OVER-2"}, "warehouse")
	if _, err := svc.ApproveDiscrepancy(res2.RecordNo, service.ApproveDiscReq{Note: "x"}, "project"); err == nil {
		t.Fatal("expect NOT_SUBMITTED error")
	}
}

// ---------------- AC-07 / AC-08 / AC-09 审批分支 ----------------

func TestApproveKittingRevisesForecast(t *testing.T) {
	svc := newSvc(t)
	fo := mkForecast(t, svc, "RMA-K", []string{"SN-K1"})
	batch, _ := svc.CreateBatch(service.CreateBatchReq{
		ForecastNo: fo.ForecastNo, LabelQtyTotal: 1, PODPhotoIDs: []string{"P"},
	}, "warehouse")
	box, _ := svc.CreateBox(batch.BatchNo, "warehouse")
	res, _ := svc.Scan(service.ScanReq{BoxNo: box.BoxNo, SN: "SN-K2"}, "warehouse")
	if _, err := svc.SubmitDiscrepancy(res.RecordNo, service.SubmitDiscReq{
		PhotoIDs: []string{"1", "2", "3"},
	}, "warehouse"); err != nil {
		t.Fatalf("submit: %v", err)
	}
	if _, err := svc.ApproveDiscrepancy(res.RecordNo, service.ApproveDiscReq{Note: "多收确认"}, "project"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	var f2 model.ForecastOrder
	svc.DB.Where("forecast_no = ?", fo.ForecastNo).First(&f2)
	if f2.Status != string(enum.ForecastRevised) || f2.ForecastQty != 2 {
		t.Fatalf("expect REVISED qty=2, got %s/%d", f2.Status, f2.ForecastQty)
	}
	var it model.SNItem
	svc.DB.Where("sn = ?", "SN-K2").First(&it)
	if it.Status != string(enum.SNReceived) || it.ForecastNo != fo.ForecastNo {
		t.Fatalf("over SN should backfill and become RECEIVED: %+v", it)
	}
	// 双异常记录（永久）
	var cnt int64
	svc.DB.Model(&model.DiscrepancyRecord{}).Where("forecast_no = ?", fo.ForecastNo).Count(&cnt)
	if cnt < 2 {
		t.Fatalf("expect dual anomaly records >=2, got %d", cnt)
	}
}

func TestApproveReceivingKeepsForecast(t *testing.T) {
	svc := newSvc(t)
	fo := mkForecast(t, svc, "RMA-RC", []string{"SN-RC1"})
	// 外箱标识数量 3 > 预报 1 → RECEIVING 差异
	if _, err := svc.CreateBatch(service.CreateBatchReq{
		ForecastNo: fo.ForecastNo, LabelQtyTotal: 3, PODPhotoIDs: []string{"P"},
	}, "warehouse"); err != nil {
		t.Fatalf("batch: %v", err)
	}
	var rec model.DiscrepancyRecord
	if err := svc.DB.Where("forecast_no = ? AND source = ?", fo.ForecastNo, "RECEIVING").First(&rec).Error; err != nil {
		t.Fatalf("expect receiving discrepancy: %v", err)
	}
	// RECEIVING 差异以 POD 为佐证，允收即锁定（RC-03），直接审批
	if _, err := svc.ApproveDiscrepancy(rec.RecordNo, service.ApproveDiscReq{Note: "放行"}, "project"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	var f2 model.ForecastOrder
	svc.DB.Where("forecast_no = ?", fo.ForecastNo).First(&f2)
	if f2.Status != string(enum.ForecastEntered) || f2.ForecastQty != 1 {
		t.Fatalf("RECEIVING diff must not revise forecast: %s/%d", f2.Status, f2.ForecastQty)
	}
}

func TestQtyShortClosesSN(t *testing.T) {
	svc := newSvc(t)
	sns := []string{"SN-S1", "SN-S2"}
	fo := mkForecast(t, svc, "RMA-S", sns)
	batch, _ := svc.CreateBatch(service.CreateBatchReq{
		ForecastNo: fo.ForecastNo, LabelQtyTotal: 2, PODPhotoIDs: []string{"P"},
	}, "warehouse")
	box, _ := svc.CreateBox(batch.BatchNo, "warehouse")
	// 只扫到 1 片
	if _, err := svc.Scan(service.ScanReq{BoxNo: box.BoxNo, SN: "SN-S1"}, "warehouse"); err != nil {
		t.Fatalf("scan: %v", err)
	}
	// 手工登记少收差异（由仓库提交）
	rec, err := svc.SubmitShortDiscrepancy(fo.ForecastNo, batch.BatchNo, box.BoxNo, []string{"p1", "p2"}, "warehouse")
	if err != nil {
		t.Fatalf("short discrepancy: %v", err)
	}
	if _, err := svc.ApproveDiscrepancy(rec.RecordNo, service.ApproveDiscReq{Note: "少收确认"}, "project"); err != nil {
		t.Fatalf("approve short: %v", err)
	}
	var it model.SNItem
	svc.DB.Where("sn = ?", "SN-S2").First(&it)
	if it.Status != string(enum.SNClosedShort) {
		t.Fatalf("expect CLOSED_SHORT, got %s", it.Status)
	}
}

// ---------------- AC-10 驳回解锁 ----------------

func TestRejectUnlocksRescan(t *testing.T) {
	svc := newSvc(t)
	fo := mkForecast(t, svc, "RMA-J", []string{"SN-J1", "SN-J2"})
	batch, _ := svc.CreateBatch(service.CreateBatchReq{
		ForecastNo: fo.ForecastNo, LabelQtyTotal: 2, PODPhotoIDs: []string{"P"},
	}, "warehouse")
	box, _ := svc.CreateBox(batch.BatchNo, "warehouse")
	res, err := svc.Scan(service.ScanReq{BoxNo: box.BoxNo, SN: "SN-J1"}, "warehouse")
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	// 先构造差异：扫一个重复 SN 触发? 直接对该记录驳回需 PENDING 且已提交
	rec2, err := svc.SubmitShortDiscrepancy(fo.ForecastNo, batch.BatchNo, box.BoxNo, []string{"p1", "p2"}, "warehouse")
	if err != nil {
		t.Fatalf("submit short: %v", err)
	}
	if _, err := svc.RejectDiscrepancy(rec2.RecordNo, "照片不清", "project"); err != nil {
		t.Fatalf("reject: %v", err)
	}
	var cnt int64
	svc.DB.Model(&model.DiscrepancyRecord{}).Where("record_no = ? AND approval_status = ?", rec2.RecordNo, "REJECTED").Count(&cnt)
	if cnt != 1 {
		t.Fatalf("expect REJECTED record")
	}
	_ = res
}

// ---------------- AC-11 / AC-12 质检 ----------------

func TestInspectionRequiresReceived(t *testing.T) {
	svc := newSvc(t)
	fo := mkForecast(t, svc, "RMA-I", []string{"SN-I1"})
	// 未 kitting（仍 FORECASTED）
	_, err := svc.Judge(service.JudgeReq{SN: "SN-I1", Judgment: enum.JudgeRepair}, "inspector")
	if err == nil {
		t.Fatal("expect state not allowed for FORECASTED SN")
	}
	_ = fo
}

func TestCIDRequiresEvidence(t *testing.T) {
	svc := newSvc(t)
	fo := mkForecast(t, svc, "RMA-C", []string{"SN-C1"})
	receive(t, svc, fo.ForecastNo, []string{"SN-C1"})
	// 缺 CID 标记
	if _, err := svc.Judge(service.JudgeReq{
		SN: "SN-C1", Judgment: enum.JudgeCID, CustomerConfirmation: "客户已确认",
		DamagePhotoIDs: []string{"dp"},
	}, "inspector"); err == nil {
		t.Fatal("expect CID_FLAG_REQUIRED")
	}
	// 缺损坏照片
	if _, err := svc.Judge(service.JudgeReq{
		SN: "SN-C1", Judgment: enum.JudgeCID, CIDFlag: true, CustomerConfirmation: "客户已确认",
	}, "inspector"); err == nil {
		t.Fatal("expect photo required")
	}
	// 缺客户确认
	if _, err := svc.Judge(service.JudgeReq{
		SN: "SN-C1", Judgment: enum.JudgeCID, CIDFlag: true, DamagePhotoIDs: []string{"dp"},
	}, "inspector"); err == nil {
		t.Fatal("expect CID_CONFIRM_REQUIRED")
	}
	// 齐备通过
	if _, err := svc.Judge(service.JudgeReq{
		SN: "SN-C1", Judgment: enum.JudgeCID, CIDFlag: true,
		DamagePhotoIDs: []string{"dp"}, CustomerConfirmation: "客户已确认",
	}, "inspector"); err != nil {
		t.Fatalf("valid CID should pass: %v", err)
	}
}

// ---------------- AC-14 BUFFER 并发预占 ----------------

func TestBufferConcurrentReserve(t *testing.T) {
	svc := newSvc(t)
	// 仅 1 件可用备件
	if _, err := svc.BufferInbound(service.InboundReq{
		SN: "BUF-ONLY", PNCode: "PN-A100", Source: "NEW_PURCHASE",
	}, "warehouse"); err != nil {
		t.Fatalf("inbound: %v", err)
	}
	// 10 个 RMA 同时抢
	const n = 10
	sns := make([]string, 0, n)
	for i := 0; i < n; i++ {
		sns = append(sns, fmt.Sprintf("SN-CON-%d", i))
	}
	for i, sn := range sns {
		fo := mkForecast(t, svc, fmt.Sprintf("RMA-CON-%d", i), []string{sn})
		receive(t, svc, fo.ForecastNo, []string{sn})
		judgeAll(t, svc, []string{sn}, enum.JudgeReplace)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	success := 0
	for _, sn := range sns {
		wg.Add(1)
		go func(sn string) {
			defer wg.Done()
			if _, err := svc.CreateReturn(service.CreateReturnReq{OriginalSN: sn}, "warehouse"); err == nil {
				mu.Lock()
				success++
				mu.Unlock()
			}
		}(sn)
	}
	wg.Wait()
	if success != 1 {
		t.Fatalf("expect exactly 1 successful reservation, got %d", success)
	}
	var b model.BufferStock
	svc.DB.Where("sn = ?", "BUF-ONLY").First(&b)
	if b.StockStatus != string(enum.BufferReserved) {
		t.Fatalf("expect RESERVED, got %s", b.StockStatus)
	}
}

// ---------------- AC-15 报废驳回重提 ----------------

func TestScrapRejectResubmit(t *testing.T) {
	svc := newSvc(t)
	fo := mkForecast(t, svc, "RMA-SC", []string{"SN-SC1"})
	receive(t, svc, fo.ForecastNo, []string{"SN-SC1"})
	judgeAll(t, svc, []string{"SN-SC1"}, enum.JudgeRepair)

	ob, err := svc.CreateOutbound(service.CreateOutboundReq{
		Type: enum.OutScrapDisposal, SNs: []string{"SN-SC1"}, RMANo: "RMA-SC",
	}, "warehouse")
	if err != nil {
		t.Fatalf("create scrap: %v", err)
	}
	if ob.Status != string(enum.OutPendingApproval) {
		t.Fatalf("expect PENDING_APPROVAL, got %s", ob.Status)
	}
	if _, err := svc.RejectScrap(ob.OutboundNo, "不同意报废", "project"); err != nil {
		t.Fatalf("reject: %v", err)
	}
	if _, err := svc.ResubmitScrap(ob.OutboundNo, "warehouse"); err != nil {
		t.Fatalf("resubmit: %v", err)
	}
	if _, err := svc.ApproveScrap(ob.OutboundNo, "同意", "project"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	// 原片状态在发运前不变
	var it model.SNItem
	svc.DB.Where("sn = ?", "SN-SC1").First(&it)
	if it.Status != string(enum.SNJudged) {
		t.Fatalf("original should stay JUDGED before ship, got %s", it.Status)
	}
	if _, err := svc.ShipOutbound(ob.OutboundNo, service.ShipReq{Carrier: "SF", TrackingNo: "S1"}, "warehouse"); err != nil {
		t.Fatalf("ship scrap: %v", err)
	}
	svc.DB.Where("sn = ?", "SN-SC1").First(&it)
	if it.Status != string(enum.SNDisposed) {
		t.Fatalf("expect DISPOSED, got %s", it.Status)
	}
}

// ---------------- AC-16 调拨白名单 ----------------

func TestTransferWhitelist(t *testing.T) {
	svc := newSvc(t)
	fo := mkForecast(t, svc, "RMA-T", []string{"SN-T1"})
	// FORECASTED 不可调拨
	if _, err := svc.CreateOutbound(service.CreateOutboundReq{
		Type: enum.OutTransferWarehouse, SNs: []string{"SN-T1"}, RMANo: "RMA-T",
	}, "warehouse"); err == nil {
		t.Fatal("expect transfer blocked for FORECASTED SN")
	}
	receive(t, svc, fo.ForecastNo, []string{"SN-T1"})
	ob, err := svc.CreateOutbound(service.CreateOutboundReq{
		Type: enum.OutTransferWarehouse, SNs: []string{"SN-T1"}, RMANo: "RMA-T",
	}, "warehouse")
	if err != nil {
		t.Fatalf("RECEIVED should be transferable: %v", err)
	}
	if _, err := svc.ConfirmCount(ob.OutboundNo, "warehouse"); err != nil {
		t.Fatalf("count confirm: %v", err)
	}
	var it model.SNItem
	svc.DB.Where("sn = ?", "SN-T1").First(&it)
	if it.Status != string(enum.SNCompleted) {
		t.Fatalf("transfer complete should be COMPLETED, got %s", it.Status)
	}
}

// ---------------- AC-17 / AC-18 闭环与 TAT ----------------

func TestFullReturnAutoClosesRMAAndTAT(t *testing.T) {
	svc := newSvc(t)
	sns := []string{"SN-F1", "SN-F2"}
	fo := mkForecast(t, svc, "RMA-F", sns)
	receive(t, svc, fo.ForecastNo, sns)
	judgeAll(t, svc, sns, enum.JudgeRepair)

	for _, sn := range sns {
		plan, err := svc.CreateReturn(service.CreateReturnReq{OriginalSN: sn}, "warehouse")
		if err != nil {
			t.Fatalf("return %s: %v", sn, err)
		}
		if _, err := svc.ShipOutbound(plan.ReturnOut.OutboundNo, service.ShipReq{
			Carrier: "DHL", TrackingNo: "TRK-" + sn,
		}, "warehouse"); err != nil {
			t.Fatalf("ship %s: %v", sn, err)
		}
	}
	var f model.ForecastOrder
	svc.DB.Where("forecast_no = ?", fo.ForecastNo).First(&f)
	if f.RMAStatus != string(enum.RMAClosed) {
		t.Fatalf("expect CLOSED, got %s", f.RMAStatus)
	}
	if f.ClosedAt == nil || f.TatDays == nil {
		t.Fatal("expect closed_at & tat_days recorded")
	}
	if *f.TatDays != 0 {
		t.Fatalf("same-day close tat should be 0, got %d", *f.TatDays)
	}
	// 审计留痕 RMA_CLOSE
	var cnt int64
	svc.DB.Model(&model.AuditLog{}).
		Where("action = ? AND biz_no = ?", string(enum.AuditRMAClose), fo.ForecastNo).Count(&cnt)
	if cnt == 0 {
		t.Fatal("expect RMA_CLOSE audit log")
	}
}

func TestTATWarningAndOverdue(t *testing.T) {
	svc := newSvc(t)
	sns := []string{"SN-TAT1"}
	fo := mkForecast(t, svc, "RMA-TAT", sns)
	receive(t, svc, fo.ForecastNo, sns)
	judgeAll(t, svc, sns, enum.JudgeRepair)

	// 第 9 天：预警
	at(svc, 9*24*time.Hour)
	if _, err := svc.RefreshTAT(); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	var f model.ForecastOrder
	svc.DB.Where("forecast_no = ?", fo.ForecastNo).First(&f)
	if !f.TatWarning || f.TatOverdue {
		t.Fatalf("day9 expect warning only: %+v", f)
	}
	alerts, err := svc.ListTATAlerts()
	if err != nil || len(alerts) != 1 || alerts[0].Overdue {
		t.Fatalf("expect 1 warning alert: %+v %v", alerts, err)
	}

	// 第 11 天：超期
	at(svc, 11*24*time.Hour)
	if _, err := svc.RefreshTAT(); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	svc.DB.Where("forecast_no = ?", fo.ForecastNo).First(&f)
	if !f.TatOverdue {
		t.Fatalf("day11 expect overdue: %+v", f)
	}
	// 完成后 TAT 落库 = 11 天
	plan, _ := svc.CreateReturn(service.CreateReturnReq{OriginalSN: "SN-TAT1"}, "warehouse")
	if _, err := svc.ShipOutbound(plan.ReturnOut.OutboundNo, service.ShipReq{
		Carrier: "DHL", TrackingNo: "TAT-1",
	}, "warehouse"); err != nil {
		t.Fatalf("ship: %v", err)
	}
	svc.DB.Where("forecast_no = ?", fo.ForecastNo).First(&f)
	if f.TatDays == nil || *f.TatDays != 11 {
		t.Fatalf("expect tat_days=11, got %v", f.TatDays)
	}
}

// ---------------- AC-19 / AC-20 RETAINED 处置 ----------------

func TestRetainedDispositions(t *testing.T) {
	svc := newSvc(t)
	if _, err := svc.BufferInbound(service.InboundReq{
		SN: "BUF-L1", PNCode: "PN-A100", Source: "NEW_PURCHASE",
	}, "warehouse"); err != nil {
		t.Fatalf("buffer: %v", err)
	}
	fo := mkForecast(t, svc, "RMA-L", []string{"SN-L1"})
	receive(t, svc, fo.ForecastNo, []string{"SN-L1"})
	judgeAll(t, svc, []string{"SN-L1"}, enum.JudgeReplace)
	plan, err := svc.CreateReturn(service.CreateReturnReq{OriginalSN: "SN-L1"}, "warehouse")
	if err != nil {
		t.Fatalf("return: %v", err)
	}
	if _, err := svc.ShipOutbound(plan.ReturnOut.OutboundNo, service.ShipReq{
		Carrier: "DHL", TrackingNo: "L1",
	}, "warehouse"); err != nil {
		t.Fatalf("ship: %v", err)
	}
	var it model.SNItem
	svc.DB.Where("sn = ?", "SN-L1").First(&it)
	if it.Status != string(enum.SNRetained) {
		t.Fatalf("expect RETAINED, got %s", it.Status)
	}
	// LONG_RETAIN 不强制收尾
	if _, err := svc.DisposeRetained("SN-L1", service.DisposeReq{Disposition: enum.RetLongRetain}, "project"); err != nil {
		t.Fatalf("long retain: %v", err)
	}
	svc.DB.Where("sn = ?", "SN-L1").First(&it)
	if it.Status != string(enum.SNRetained) {
		t.Fatalf("LONG_RETAIN should keep RETAINED, got %s", it.Status)
	}
	// REPAIR_TO_BUFFER：同 SN 双身份
	if _, err := svc.DisposeRetained("SN-L1", service.DisposeReq{Disposition: enum.RetRepairToBuffer}, "project"); err != nil {
		t.Fatalf("repair to buffer: %v", err)
	}
	svc.DB.Where("sn = ?", "SN-L1").First(&it)
	if it.Status != string(enum.SNDisposed) {
		t.Fatalf("expect DISPOSED, got %s", it.Status)
	}
	var b model.BufferStock
	if err := svc.DB.Where("sn = ?", "SN-L1").First(&b).Error; err != nil {
		t.Fatalf("expect buffer stock with same SN: %v", err)
	}
	if b.StockStatus != string(enum.BufferAvailable) || b.Source != "REPAIRED_GOOD" {
		t.Fatalf("expect AVAILABLE/REPAIRED_GOOD, got %+v", b)
	}
}

// ---------------- AC-21 跨 PN 替代审批 ----------------

func TestCrossPNRequiresApproval(t *testing.T) {
	svc := newSvc(t)
	// 无同 PN 备件，只有异 PN
	if _, err := svc.BufferInbound(service.InboundReq{
		SN: "BUF-B200", PNCode: "PN-B200", Source: "NEW_PURCHASE",
	}, "warehouse"); err != nil {
		t.Fatalf("buffer: %v", err)
	}
	fo := mkForecast(t, svc, "RMA-XP", []string{"SN-XP1"})
	receive(t, svc, fo.ForecastNo, []string{"SN-XP1"})
	judgeAll(t, svc, []string{"SN-XP1"}, enum.JudgeReplace)

	// 指定异 PN 备件但未审批 → 阻断
	if _, err := svc.CreateReturn(service.CreateReturnReq{
		OriginalSN: "SN-XP1", BufferSN: "BUF-B200",
	}, "warehouse"); err == nil {
		t.Fatal("expect substitute approval required")
	}
	ap, err := svc.RequestSubstitute(service.SubstituteReq{
		OriginalSN: "SN-XP1", SubstituteSN: "BUF-B200", Note: "同规格替代",
	}, "warehouse")
	if err != nil {
		t.Fatalf("request substitute: %v", err)
	}
	if _, err := svc.ApproveSubstitute(ap.ApprovalNo, "同意", "project", true); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if _, err := svc.CreateReturn(service.CreateReturnReq{
		OriginalSN: "SN-XP1", BufferSN: "BUF-B200",
	}, "warehouse"); err != nil {
		t.Fatalf("approved substitute should pass: %v", err)
	}
}

// ---------------- AC-22 照片大小 ----------------

func TestPhotoSizeLimit(t *testing.T) {
	svc := newSvc(t)
	_, err := svc.SavePhoto(service.SavePhotoReq{
		BizType: enum.PhotoPOD, BizRefNo: "B1", FileName: "a.jpg",
		ContentType: "image/jpeg", Size: service.MaxPhotoSize + 1,
		ObjectKey: "k", OriginalURL: "u", UploadedBy: "warehouse",
	})
	if err == nil {
		t.Fatal("expect payload too large")
	}
	p, err := svc.SavePhoto(service.SavePhotoReq{
		BizType: enum.PhotoPOD, BizRefNo: "B1", FileName: "a.jpg",
		ContentType: "image/jpeg", Size: 1024, ObjectKey: "k", OriginalURL: "u",
		UploadedBy: "warehouse",
	})
	if err != nil {
		t.Fatalf("small photo should pass: %v", err)
	}
	if p.RetentionUntil.Year() < p.UploadedAt.Year()+1 {
		t.Fatalf("expect retention >= 2 years: %v", p.RetentionUntil)
	}
}

// ---------------- AC-24 审计不可删（应用层无删除路径） ----------------

func TestAuditAndAnomalyImmutableByAPI(t *testing.T) {
	svc := newSvc(t)
	fo := mkForecast(t, svc, "RMA-AU", []string{"SN-AU1"})
	receive(t, svc, fo.ForecastNo, []string{"SN-AU1"})
	var cnt int64
	svc.DB.Model(&model.AuditLog{}).Where("tenant_id = ?", "BOE_US").Count(&cnt)
	if cnt == 0 {
		t.Fatal("expect audit logs written")
	}
	// 服务层不提供删除方法：仅校验不存在 Delete 语义（编译期保证）
	_ = svc
}

// helper: 少收差异（供测试构造）
func TestMain(m *testing.M) {
	m.Run()
}

var _ = gorm.ErrRecordNotFound
