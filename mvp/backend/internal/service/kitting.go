package service

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/boe/rma/internal/enum"
	"github.com/boe/rma/internal/errs"
	"github.com/boe/rma/internal/model"
	"github.com/boe/rma/internal/seq"
)

// CreateBox 开箱建箱号（KT-07 / RC-05）：BOE+YYMMDD+3位流水。
func (s *Service) CreateBox(batchNo, operator string) (*model.BoxInfo, error) {
	var box *model.BoxInfo
	err := s.Tx(func(tx *gorm.DB) error {
		var batch model.ReceivingBatch
		if err := tx.Where("tenant_id = ? AND batch_no = ?", s.Tenant, batchNo).First(&batch).Error; err != nil {
			return errs.NotFound("允收批次 " + batchNo)
		}
		no, err := seq.NextBoxNo(tx, s.Tenant, model.SeqReceive, s.now())
		if err != nil {
			return err
		}
		box = &model.BoxInfo{
			TenantID: s.Tenant, BoxNo: no, BatchNo: batchNo, ForecastNo: batch.ForecastNo,
			Status: "OPEN", CreatedBy: operator, CreatedAt: s.now(),
		}
		return tx.Create(box).Error
	})
	if err != nil {
		return nil, err
	}
	return box, nil
}

// ScanReq kitting 扫码入参。PNCode 选填（R-006：提供了才做 PN 维度比对）。
type ScanReq struct {
	BoxNo  string `json:"box_no"`
	SN     string `json:"sn"`
	PNCode string `json:"pn_code"`
}

// ScanResp 扫码结果（实时四维校验反馈）。
type ScanResp struct {
	Result          string `json:"result"`            // OK / DISCREPANCY
	DiscrepancyType string `json:"discrepancy_type"`  // 差异类型（Result=DISCREPANCY 时）
	Message         string `json:"message"`
	SNStatus        string `json:"sn_status"`
	BoxSNCount      int64  `json:"box_sn_count"`
	RecordNo        string `json:"record_no,omitempty"`
}

// Scan kitting 逐片扫码四维校验（KT-01/02/03，DC-01）。
func (s *Service) Scan(req ScanReq, operator string) (*ScanResp, error) {
	sn := normSN(req.SN)
	if sn == "" {
		return nil, errs.BadRequest("INVALID_SN", "SN 不能为空")
	}
	resp := &ScanResp{}
	err := s.Tx(func(tx *gorm.DB) error {
		var box model.BoxInfo
		if err := tx.Where("tenant_id = ? AND box_no = ?", s.Tenant, req.BoxNo).First(&box).Error; err != nil {
			return errs.NotFound("箱号 " + req.BoxNo)
		}
		if box.Status != "OPEN" {
			return errs.Conflict("BOX_SEALED", "箱号 "+box.BoxNo+" 已封箱，不能再扫码")
		}
		fo, err := s.getForecast(tx, box.ForecastNo)
		if err != nil {
			return err
		}

		// 重复扫码校验
		var dupCount int64
		if err := tx.Model(&model.BoxSNItem{}).
			Where("tenant_id = ? AND sn = ?", s.Tenant, sn).Count(&dupCount).Error; err != nil {
			return err
		}
		if dupCount > 0 {
			return errs.Conflict("SN_DUPLICATED", "该 SN 已扫过（箱号已存在），无需重复扫描")
		}

		// ① SN 是否在预报清单内
		var fsi model.ForecastSNItem
		foundErr := tx.Where("tenant_id = ? AND forecast_no = ? AND sn = ?", s.Tenant, fo.ForecastNo, sn).
			First(&fsi).Error
		inForecast := foundErr == nil
		if foundErr != nil && foundErr != gorm.ErrRecordNotFound {
			return foundErr
		}

		var discType enum.DiscrepancyType
		if inForecast {
			// ② 料号一致（仅在提供实际 PN 时比对，R-006）
			if req.PNCode != "" && trimAll(req.PNCode) != fsi.PNCode {
				discType = enum.DiscPNMismatch
			}
		} else {
			// SN 不在本预报：区分 RMA 不一致 / 未预报
			var exist model.SNItem
			e := tx.Where("tenant_id = ? AND sn = ?", s.Tenant, sn).First(&exist).Error
			if e == nil && exist.RMANo != "" && exist.RMANo != fo.RMANo {
				discType = enum.DiscRMAMismatch
			} else {
				discType = enum.DiscSNNotInForecast
			}
			if e != nil && e != gorm.ErrRecordNotFound {
				return e
			}
			// ④ 数量一致：已达/超预报数量则记多收
			var arrived int64
			if err := tx.Model(&model.SNItem{}).
				Where("tenant_id = ? AND forecast_no = ? AND status NOT IN ?", s.Tenant, fo.ForecastNo,
					[]string{string(enum.SNForecasted), string(enum.SNClosedShort)}).
				Count(&arrived).Error; err != nil {
				return err
			}
			if arrived >= int64(fo.ForecastQty) {
				discType = enum.DiscQtyOver
			}
		}

		// 写入箱内 SN（按实物接收，不阻断入库 DC-01）
		if err := tx.Create(&model.BoxSNItem{TenantID: s.Tenant, BoxNo: box.BoxNo, SN: sn}).Error; err != nil {
			return err
		}

		if discType == "" {
			// 四维校验通过
			it, err := s.getSN(tx, sn)
			if err != nil {
				// 预报存在但 sn_item 缺失：补建
				if err := tx.Create(&model.SNItem{
					TenantID: s.Tenant, SN: sn, Status: string(enum.SNForecasted),
					RMANo: fo.RMANo, PNCode: fsi.PNCode, ForecastNo: fo.ForecastNo, Owner: operator,
				}).Error; err != nil {
					return err
				}
				it, err = s.getSN(tx, sn)
				if err != nil {
					return err
				}
			}
			if enum.SNStatus(it.Status) == enum.SNDiscrepancy {
				if _, err := s.transit(tx, sn, enum.SNReceived, statemachineCtxFor(""), operator); err != nil {
					return err
				}
			} else if enum.SNStatus(it.Status) == enum.SNForecasted {
				if _, err := s.transit(tx, sn, enum.SNReceived, statemachineCtxFor(""), operator); err != nil {
					return err
				}
			}
			if err := s.setSNFields(tx, sn, map[string]any{
				"batch_no": box.BatchNo, "box_no": box.BoxNo, "pn_code": fsi.PNCode, "rma_no": fo.RMANo,
			}); err != nil {
				return err
			}
			resp.Result = "OK"
			resp.Message = "校验通过，已接收"
			resp.SNStatus = string(enum.SNReceived)
		} else {
			// 差异：SN 置 DISCREPANCY，生成差异记录
			it, err := s.getSN(tx, sn)
			if err != nil {
				// 未预报实物：新建 sn_item（forecast_no 为空，SN-05）
				if err := tx.Create(&model.SNItem{
					TenantID: s.Tenant, SN: sn, Status: string(enum.SNDiscrepancy),
					RMANo: fo.RMANo, PNCode: trimAll(req.PNCode), ForecastNo: "", Owner: operator,
				}).Error; err != nil {
					return err
				}
			} else {
				if enum.SNStatus(it.Status) == enum.SNForecasted {
					if _, err := s.transit(tx, sn, enum.SNDiscrepancy, statemachineCtxFor(""), operator); err != nil {
						return err
					}
				}
				if err := s.setSNFields(tx, sn, map[string]any{
					"batch_no": box.BatchNo, "box_no": box.BoxNo,
				}); err != nil {
					return err
				}
			}
			rec, err := s.createDiscrepancy(tx, discInput{
				Source: enum.SourceKitting, Type: discType, ForecastNo: fo.ForecastNo,
				BatchNo: box.BatchNo, BoxNo: box.BoxNo, SN: sn,
				PNCode: trimAll(req.PNCode), RMANo: fo.RMANo, QtyDiff: qtyDiffFor(discType),
			}, operator)
			if err != nil {
				return err
			}
			resp.Result = "DISCREPANCY"
			resp.DiscrepancyType = string(discType)
			resp.Message = discrepancyMessage(discType)
			resp.SNStatus = string(enum.SNDiscrepancy)
			resp.RecordNo = rec.RecordNo
		}
		var cnt int64
		if err := tx.Model(&model.BoxSNItem{}).
			Where("tenant_id = ? AND box_no = ?", s.Tenant, box.BoxNo).Count(&cnt).Error; err != nil {
			return err
		}
		resp.BoxSNCount = cnt
		return nil
	})
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func qtyDiffFor(t enum.DiscrepancyType) int {
	switch t {
	case enum.DiscQtyOver, enum.DiscSNNotInForecast:
		return 1
	case enum.DiscQtyShort:
		return -1
	}
	return 0
}

func discrepancyMessage(t enum.DiscrepancyType) string {
	switch t {
	case enum.DiscSNNotInForecast:
		return "该 SN 不在本批次预报清单，已按实物接收并记录差异。请拍照（箱内物+箱标含数量+实物PN/SN）后提交审批"
	case enum.DiscQtyOver:
		return "已超出预报数量，多收部分需拍照：箱内物+箱标(含数量)+多收实物PN/SN"
	case enum.DiscPNMismatch:
		return "料号与预报不一致，已按实物接收并记录差异，请拍照后提交审批"
	case enum.DiscRMAMismatch:
		return "该 SN 归属其他 RMA，已记录差异，请拍照后提交审批"
	case enum.DiscQtyShort:
		return "少收，请拍照（箱内物+箱标含数量）并提交审批"
	}
	return "差异"
}

type discInput struct {
	Source          string
	Type            enum.DiscrepancyType
	ForecastNo      string
	BatchNo         string
	BoxNo           string
	SN              string
	PNCode          string
	RMANo           string
	QtyDiff         int
	MissingSNList   []string
	PhotoIDs        []string
	DiscoveredBy    string
}

// createDiscrepancy 生成差异记录（提交前 locked=false，须补照片后 submit）。
func (s *Service) createDiscrepancy(tx *gorm.DB, in discInput, operator string) (*model.DiscrepancyRecord, error) {
	no, err := seq.NextBizNo(tx, s.Tenant, "DISCREPANCY", "D", 4, s.now())
	if err != nil {
		return nil, err
	}
	rec := &model.DiscrepancyRecord{
		TenantID: s.Tenant, RecordNo: no,
		Category: string(enum.AnomalyArrival), Source: in.Source,
		DiscrepancyType: string(in.Type), ForecastNo: in.ForecastNo,
		BatchNo: in.BatchNo, BoxNo: in.BoxNo, SN: in.SN, PNCode: in.PNCode, RMANo: in.RMANo,
		MissingSNList: in.MissingSNList, QtyDiff: in.QtyDiff, PhotoIDs: in.PhotoIDs,
		ApprovalStatus: string(enum.ApprovalPending),
		DiscoveredBy:   operator, DiscoveredAt: s.now(), Locked: false,
	}
	if err := tx.Create(rec).Error; err != nil {
		return nil, err
	}
	return rec, nil
}

// SealBox 封箱（KT-04/KT-08）：锁定箱与箱内 SN，生成箱标打印记录。
func (s *Service) SealBox(boxNo, operator string) (*model.BoxInfo, error) {
	var box model.BoxInfo
	err := s.Tx(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ? AND box_no = ?", s.Tenant, boxNo).First(&box).Error; err != nil {
			return errs.NotFound("箱号 " + boxNo)
		}
		if box.Status == "SEALED" {
			return errs.Conflict("BOX_SEALED", "箱号 "+boxNo+" 已封箱")
		}
		var items []model.BoxSNItem
		if err := tx.Where("tenant_id = ? AND box_no = ?", s.Tenant, boxNo).Find(&items).Error; err != nil {
			return err
		}
		if len(items) == 0 {
			return errs.BadRequest("EMPTY_BOX", "空箱不能封箱")
		}
		sns := make([]string, 0, len(items))
		for _, it := range items {
			sns = append(sns, it.SN)
			if err := tx.Model(&model.SNItem{}).
				Where("tenant_id = ? AND sn = ?", s.Tenant, it.SN).
				Update("locked", true).Error; err != nil {
				return err
			}
		}
		now := s.now()
		box.Status = "SEALED"
		box.SNList = sns
		box.SealedBy = operator
		box.SealedAt = &now
		box.LabelPrintLog = model.JSONMap{"printed_at": now.Format("2006-01-02 15:04:05"), "operator": operator}
		if err := tx.Save(&box).Error; err != nil {
			return err
		}
		return s.audit(tx, enum.AuditBoxSeal, "box_info", boxNo, "status", "OPEN", "SEALED", "封箱并打印箱标", operator, "")
	})
	if err != nil {
		return nil, err
	}
	// 封箱后尝试推进 RMA（到齐判定）
	if err := s.ReconcileRMA(box.ForecastNo, operator); err != nil {
		return nil, err
	}
	return &box, nil
}

// BoxDetail 返回箱详情（含 SN 明细）。
func (s *Service) BoxDetail(boxNo string) (map[string]any, error) {
	var box model.BoxInfo
	if err := s.DB.Where("tenant_id = ? AND box_no = ?", s.Tenant, boxNo).First(&box).Error; err != nil {
		return nil, errs.NotFound("箱号 " + boxNo)
	}
	var items []model.BoxSNItem
	if err := s.DB.Where("tenant_id = ? AND box_no = ?", s.Tenant, boxNo).Find(&items).Error; err != nil {
		return nil, err
	}
	return map[string]any{"box": box, "sn_items": items}, nil
}

// ListBoxes 按批次列箱。
func (s *Service) ListBoxes(batchNo string, page, size int) (PageResult, error) {
	q := s.DB.Model(&model.BoxInfo{}).Where("tenant_id = ?", s.Tenant)
	if batchNo != "" {
		q = q.Where("batch_no = ?", batchNo)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return PageResult{}, err
	}
	var rows []model.BoxInfo
	q2, p, sz := paged(q, page, size)
	if err := q2.Order("created_at DESC").Find(&rows).Error; err != nil {
		return PageResult{}, err
	}
	return wrapPage(rows, total, p, sz), nil
}

// ReceiveLabelHTML 收货箱标（箱号 + SN 明细，浏览器打印）。
func (s *Service) ReceiveLabelHTML(boxNo string) (string, error) {
	d, err := s.BoxDetail(boxNo)
	if err != nil {
		return "", err
	}
	box := d["box"].(model.BoxInfo)
	var b string
	b = fmt.Sprintf(`<div class="label"><div class="title">收货箱标 / RECEIVE LABEL</div>
<div class="row"><span>箱号 Box No.</span><b>%s</b></div>
<div class="row"><span>批次 Batch</span><b>%s</b></div>
<div class="row"><span>预报单 Forecast</span><b>%s</b></div>
<div class="sn-title">SN 明细 (%d)</div><ul>`, box.BoxNo, box.BatchNo, box.ForecastNo, len(box.SNList))
	for _, sn := range box.SNList {
		b += "<li>" + sn + "</li>"
	}
	b += `</ul></div>`
	return labelPage(box.BoxNo, b), nil
}

func labelPage(title, body string) string {
	return `<!doctype html><html><head><meta charset="utf-8"><title>` + title + `</title>
<style>
body{font-family:-apple-system,"Segoe UI",Arial,sans-serif;margin:0;padding:16px;background:#fff;color:#111}
.label{width:100mm;border:1px solid #111;padding:10px}
.title{font-size:14px;font-weight:700;border-bottom:1px dashed #111;padding-bottom:6px;margin-bottom:8px}
.row{display:flex;justify-content:space-between;font-size:12px;padding:2px 0}
.sn-title{font-size:12px;font-weight:700;margin-top:8px;border-top:1px dashed #111;padding-top:6px}
ul{margin:4px 0 0 0;padding-left:16px;font-size:11px;line-height:1.5}
@media print{@page{size:100mm auto;margin:4mm}}
</style></head><body onload="window.print()">` + body + `</body></html>`
}
