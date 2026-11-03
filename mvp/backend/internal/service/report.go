package service

import (
	"time"

	"github.com/boe/rma/internal/enum"
	"github.com/boe/rma/internal/errs"
	"github.com/boe/rma/internal/model"
)

// ---------- RP-01 按 RMA / 客户汇总 ----------

type RMASummaryRow struct {
	RMANo      string `json:"rma_no"`
	Customer   string `json:"customer"`
	ForecastNo string `json:"forecast_no"`
	SNTotal    int64  `json:"sn_total"`
	Judged     int64  `json:"judged"`
	Shipped    int64  `json:"shipped"`
	RMAStatus  string `json:"rma_status"`
	CreatedAt  string `json:"created_at"`
}

func (s *Service) ReportRMA(customer string, page, size int) (PageResult, error) {
	q := s.DB.Model(&model.ForecastOrder{}).Where("tenant_id = ?", s.Tenant)
	if customer != "" {
		q = q.Where("customer = ?", customer)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return PageResult{}, err
	}
	var rows []model.ForecastOrder
	q2, p, sz := paged(q, page, size)
	if err := q2.Order("created_at DESC").Find(&rows).Error; err != nil {
		return PageResult{}, err
	}
	out := make([]RMASummaryRow, 0, len(rows))
	for _, r := range rows {
		var snTotal, judged, shipped int64
		s.DB.Model(&model.SNItem{}).Where("tenant_id = ? AND forecast_no = ?", s.Tenant, r.ForecastNo).Count(&snTotal)
		s.DB.Model(&model.SNItem{}).Where("tenant_id = ? AND forecast_no = ? AND judgment_result <> ''", s.Tenant, r.ForecastNo).Count(&judged)
		s.DB.Model(&model.ReturnRecord{}).Where("tenant_id = ? AND forecast_no = ? AND status = ?", s.Tenant, r.ForecastNo, "SHIPPED").Count(&shipped)
		out = append(out, RMASummaryRow{
			RMANo: r.RMANo, Customer: r.Customer, ForecastNo: r.ForecastNo,
			SNTotal: snTotal, Judged: judged, Shipped: shipped, RMAStatus: r.RMAStatus,
			CreatedAt: r.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	return wrapPage(out, total, p, sz), nil
}

// ---------- RP-02 按 SKU（机型/PN）统计 ----------

type SKURow struct {
	PNCode   string `json:"pn_code"`
	ModelCode string `json:"model_code"`
	SNCount  int64  `json:"sn_count"`
}

func (s *Service) ReportSKU() ([]SKURow, error) {
	type raw struct {
		PNCode string
		Cnt    int64
	}
	var raws []raw
	if err := s.DB.Model(&model.SNItem{}).
		Select("pn_code, count(*) as cnt").
		Where("tenant_id = ?", s.Tenant).Group("pn_code").Scan(&raws).Error; err != nil {
		return nil, err
	}
	pnToModel := map[string]string{}
	var pns []model.PartNumber
	s.DB.Where("tenant_id = ?", s.Tenant).Find(&pns)
	for _, p := range pns {
		pnToModel[p.PNCode] = p.ModelCode
	}
	out := make([]SKURow, 0, len(raws))
	for _, r := range raws {
		out = append(out, SKURow{PNCode: r.PNCode, ModelCode: pnToModel[r.PNCode], SNCount: r.Cnt})
	}
	return out, nil
}

// ---------- RP-03 周期趋势 ----------

type TrendRow struct {
	Period string `json:"period"`
	Count  int64  `json:"count"`
}

// ReportTrend period: week | month
func (s *Service) ReportTrend(period string, limit int) ([]TrendRow, error) {
	if limit <= 0 || limit > 52 {
		limit = 12
	}
	var rows []TrendRow
	// 用字符串截取保证 MySQL/SQLite 一致（先取全部再在内存聚合）
	var items []model.SNItem
	if err := s.DB.Select("sn", "received_at").Where("tenant_id = ? AND received_at IS NOT NULL", s.Tenant).
		Find(&items).Error; err != nil {
		return nil, err
	}
	agg := map[string]int64{}
	for _, it := range items {
		if it.ReceivedAt == nil {
			continue
		}
		var key string
		if period == "week" {
			y, w := it.ReceivedAt.ISOWeek()
			key = time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC).Format("2006") + "-W" + pad2(w)
		} else {
			key = it.ReceivedAt.Format("2006-01")
		}
		agg[key]++
	}
	keys := make([]string, 0, len(agg))
	for k := range agg {
		keys = append(keys, k)
	}
	// 排序（字符串可比较）
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	if len(keys) > limit {
		keys = keys[len(keys)-limit:]
	}
	for _, k := range keys {
		rows = append(rows, TrendRow{Period: k, Count: agg[k]})
	}
	return rows, nil
}

func pad2(v int) string {
	if v < 10 {
		return "0" + itoa(v)
	}
	return itoa(v)
}

// ---------- RP-04 差异/异常统计 + 出库分布 ----------

type DiscStatRow struct {
	DiscrepancyType string `json:"discrepancy_type"`
	Count           int64  `json:"count"`
}

type OutboundDistRow struct {
	OutboundType string `json:"outbound_type"`
	Count        int64  `json:"count"`
	CIDCount     int64  `json:"cid_count"`
}

type ReportDiscrepancyResp struct {
	ByType      []DiscStatRow     `json:"by_type"`
	ByOutbound  []OutboundDistRow `json:"by_outbound"`
	CIDRatio    float64           `json:"cid_ratio"`
	Total       int64             `json:"total"`
}

func (s *Service) ReportDiscrepancy() (*ReportDiscrepancyResp, error) {
	var byType []DiscStatRow
	if err := s.DB.Model(&model.DiscrepancyRecord{}).
		Select("discrepancy_type, count(*) as count").
		Where("tenant_id = ?", s.Tenant).Group("discrepancy_type").Scan(&byType).Error; err != nil {
		return nil, err
	}
	type rawOb struct {
		OutboundType string
		Cnt          int64
	}
	var raws []rawOb
	if err := s.DB.Model(&model.OutboundOrder{}).
		Select("outbound_type, count(*) as cnt").
		Where("tenant_id = ?", s.Tenant).Group("outbound_type").Scan(&raws).Error; err != nil {
		return nil, err
	}
	out := &ReportDiscrepancyResp{}
	var totalSN, cidSN int64
	s.DB.Model(&model.SNItem{}).Where("tenant_id = ?", s.Tenant).Count(&totalSN)
	s.DB.Model(&model.SNItem{}).Where("tenant_id = ? AND cid_flag = ?", s.Tenant, true).Count(&cidSN)
	if totalSN > 0 {
		out.CIDRatio = float64(cidSN) / float64(totalSN)
	}
	for _, r := range raws {
		var cid int64
		if enum.OutboundType(r.OutboundType) == enum.OutShipToCustomer {
			s.DB.Model(&model.OutboundOrder{}).
				Where("tenant_id = ? AND outbound_type = ? AND cid_flag = ?", s.Tenant, r.OutboundType, true).Count(&cid)
		}
		out.ByOutbound = append(out.ByOutbound, OutboundDistRow{
			OutboundType: r.OutboundType, Count: r.Cnt, CIDCount: cid,
		})
		out.Total += r.Cnt
	}
	out.ByType = byType
	return out, nil
}

// ---------- RP-05 明细报表 ----------

type DetailRow struct {
	SN             string `json:"sn"`
	PNCode         string `json:"pn_code"`
	RMANo          string `json:"rma_no"`
	Status         string `json:"status"`
	InboundAt      string `json:"inbound_at"`
	OutboundAt     string `json:"outbound_at"`
	JudgmentResult string `json:"judgment_result"`
	Owner          string `json:"owner"`
	ReturnMethod   string `json:"return_method"`
}

func (s *Service) ReportDetail(status, rmaNo string, page, size int) (PageResult, error) {
	q := s.DB.Model(&model.SNItem{}).Where("tenant_id = ?", s.Tenant)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if rmaNo != "" {
		q = q.Where("rma_no = ?", rmaNo)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return PageResult{}, err
	}
	var rows []model.SNItem
	q2, p, sz := paged(q, page, size)
	if err := q2.Order("updated_at DESC").Find(&rows).Error; err != nil {
		return PageResult{}, err
	}
	out := make([]DetailRow, 0, len(rows))
	for _, r := range rows {
		in, outAt := "", ""
		if r.ReceivedAt != nil {
			in = r.ReceivedAt.Format("2006-01-02 15:04:05")
		}
		if r.OutboundAt != nil {
			outAt = r.OutboundAt.Format("2006-01-02 15:04:05")
		}
		out = append(out, DetailRow{
			SN: r.SN, PNCode: r.PNCode, RMANo: r.RMANo, Status: r.Status,
			InboundAt: in, OutboundAt: outAt, JudgmentResult: r.JudgmentResult,
			Owner: r.Owner, ReturnMethod: r.ReturnMethod,
		})
	}
	return wrapPage(out, total, p, sz), nil
}

// ---------- RP-06 TAT 报表 ----------

type TATBucket struct {
	Label string `json:"label"`
	Count int64  `json:"count"`
}

type TATReport struct {
	ClosedCount  int64       `json:"closed_count"`
	AvgDays      float64     `json:"avg_days"`
	MaxDays      int         `json:"max_days"`
	OverdueCount int64       `json:"overdue_count"`
	OverdueRate  float64     `json:"overdue_rate"`
	Buckets      []TATBucket `json:"buckets"`
}

// ReportTAT TAT 平均/分布/超期率（≤5 / 6-8 / 9-10 / >10 四档）。
func (s *Service) ReportTAT(customer string) (*TATReport, error) {
	q := s.DB.Model(&model.ForecastOrder{}).
		Where("tenant_id = ? AND closed_at IS NOT NULL AND tat_days IS NOT NULL", s.Tenant)
	if customer != "" {
		q = q.Where("customer = ?", customer)
	}
	var rows []model.ForecastOrder
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	rep := &TATReport{Buckets: []TATBucket{
		{Label: "≤5 天", Count: 0}, {Label: "6-8 天", Count: 0},
		{Label: "9-10 天", Count: 0}, {Label: ">10 天", Count: 0},
	}}
	sum := 0
	for _, r := range rows {
		d := *r.TatDays
		sum += d
		if d > rep.MaxDays {
			rep.MaxDays = d
		}
		switch {
		case d <= 5:
			rep.Buckets[0].Count++
		case d <= 8:
			rep.Buckets[1].Count++
		case d <= 10:
			rep.Buckets[2].Count++
		default:
			rep.Buckets[3].Count++
			rep.OverdueCount++
		}
	}
	rep.ClosedCount = int64(len(rows))
	if len(rows) > 0 {
		rep.AvgDays = float64(sum) / float64(len(rows))
		rep.OverdueRate = float64(rep.OverdueCount) / float64(len(rows))
	}
	return rep, nil
}

// TATAlertRow TAT 预警行（第 8 天预警 / >10 标红）。
type TATAlertRow struct {
	ForecastNo  string `json:"forecast_no"`
	RMANo       string `json:"rma_no"`
	Customer    string `json:"customer"`
	RMAStatus   string `json:"rma_status"`
	ElapsedDays int    `json:"elapsed_days"`
	Warning     bool   `json:"warning"`
	Overdue     bool   `json:"overdue"`
}

// ListTATAlerts 超期预警列表（RT-08）。
func (s *Service) ListTATAlerts() ([]TATAlertRow, error) {
	var rows []model.ForecastOrder
	if err := s.DB.Where("tenant_id = ? AND full_received_at IS NOT NULL AND closed_at IS NULL", s.Tenant).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := []TATAlertRow{}
	for _, r := range rows {
		d := TatDays(*r.FullReceivedAt, s.now())
		if d < TATWarningDays {
			continue
		}
		out = append(out, TATAlertRow{
			ForecastNo: r.ForecastNo, RMANo: r.RMANo, Customer: r.Customer,
			RMAStatus: r.RMAStatus, ElapsedDays: d,
			Warning: d >= TATWarningDays, Overdue: d > TATLimitDays,
		})
	}
	return out, nil
}

// ---------- SN 全生命周期轨迹（SN-03） ----------

func (s *Service) SNTrace(sn string) (map[string]any, error) {
	sn = normSN(sn)
	var it model.SNItem
	if err := s.DB.Where("tenant_id = ? AND sn = ?", s.Tenant, sn).First(&it).Error; err != nil {
		return nil, errs.NotFound("SN " + sn)
	}
	var rounds []model.SNItemRound
	s.DB.Where("tenant_id = ? AND sn = ?", s.Tenant, sn).Order("round_no").Find(&rounds)
	var discs []model.DiscrepancyRecord
	s.DB.Where("tenant_id = ? AND sn = ?", s.Tenant, sn).Order("discovered_at").Find(&discs)
	var inspects []model.InspectionDisposition
	s.DB.Where("tenant_id = ? AND sn = ?", s.Tenant, sn).Order("inspection_at").Find(&inspects)
	var rets []model.ReturnRecord
	s.DB.Where("tenant_id = ? AND original_sn = ?", s.Tenant, sn).Find(&rets)
	var photos []model.PhotoEvidence
	s.DB.Where("tenant_id = ? AND sn = ?", s.Tenant, sn).Find(&photos)
	var audits []model.AuditLog
	s.DB.Where("tenant_id = ? AND biz_no = ?", s.Tenant, sn).Order("created_at").Find(&audits)
	return map[string]any{
		"sn_item": it, "rounds": rounds, "discrepancies": discs,
		"inspections": inspects, "returns": rets, "photos": photos, "audit": audits,
	}, nil
}

// ---------- 审计日志查询 ----------

func (s *Service) ListAuditLogs(action, bizNo string, page, size int) (PageResult, error) {
	q := s.DB.Model(&model.AuditLog{}).Where("tenant_id = ?", s.Tenant)
	if action != "" {
		q = q.Where("action = ?", action)
	}
	if bizNo != "" {
		q = q.Where("biz_no = ?", bizNo)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return PageResult{}, err
	}
	var rows []model.AuditLog
	q2, p, sz := paged(q, page, size)
	if err := q2.Order("created_at DESC").Find(&rows).Error; err != nil {
		return PageResult{}, err
	}
	return wrapPage(rows, total, p, sz), nil
}
