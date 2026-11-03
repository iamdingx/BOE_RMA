package service

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/boe/rma/internal/enum"
	"github.com/boe/rma/internal/statemachine"
	"github.com/boe/rma/internal/errs"
	"github.com/boe/rma/internal/model"
)

// PhotoRules 差异照片数量下限（DC-07：多收≥3；DC-08：少收≥2；其余≥1）。
func PhotoRules(t enum.DiscrepancyType) int {
	switch t {
	case enum.DiscQtyOver, enum.DiscSNNotInForecast:
		return 3
	case enum.DiscQtyShort:
		return 2
	default:
		return 1
	}
}

// SubmitDiscReq 差异提交（补佐证照片后锁定进入审核流）。
type SubmitDiscReq struct {
	PhotoIDs []string `json:"photo_ids"`
	Note     string   `json:"note"`
}

// SubmitDiscrepancy 提交差异（DC-02/DC-03）：校验照片数量并锁定。
func (s *Service) SubmitDiscrepancy(recordNo string, req SubmitDiscReq, operator string) (*model.DiscrepancyRecord, error) {
	var rec model.DiscrepancyRecord
	err := s.Tx(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ? AND record_no = ?", s.Tenant, recordNo).First(&rec).Error; err != nil {
			return errs.NotFound("差异记录 " + recordNo)
		}
		if rec.ApprovalStatus != string(enum.ApprovalPending) {
			return errs.ErrStateNotAllowed("提交差异", rec.ApprovalStatus)
		}
		if rec.Locked {
			return errs.Conflict("ALREADY_SUBMITTED", "差异已提交，不可重复提交")
		}
		need := PhotoRules(enum.DiscrepancyType(rec.DiscrepancyType))
		if len(req.PhotoIDs) < need {
			return errs.ErrPhotoRequired(need)
		}
		updates := map[string]any{"photo_ids": model.StrList(req.PhotoIDs), "locked": true}
		// 少收：自动列出未到货 SN 清单（DC-09）
		if enum.DiscrepancyType(rec.DiscrepancyType) == enum.DiscQtyShort && rec.ForecastNo != "" {
			missing, err := s.computeMissingSNs(tx, rec.ForecastNo)
			if err != nil {
				return err
			}
			rec.MissingSNList = missing
			updates["missing_sn_list"] = model.StrList(missing)
			updates["qty_diff"] = -len(missing)
		}
		if err := tx.Model(&model.DiscrepancyRecord{}).
			Where("tenant_id = ? AND record_no = ?", s.Tenant, recordNo).Updates(updates).Error; err != nil {
			return err
		}
		rec.PhotoIDs = req.PhotoIDs
		rec.Locked = true
		var extras []model.SNItem
		if err := tx.Where("tenant_id = ? AND forecast_no = ?", s.Tenant, rec.ForecastNo).Find(&extras).Error; err != nil {
			return err
		}
		return s.audit(tx, enum.AuditControlledCorrection, "discrepancy_record", recordNo, "photo_ids",
			"", fmt.Sprint(req.PhotoIDs), "差异提交（"+req.Note+"）", operator, "")
	})
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

// computeMissingSNs 计算未到货的预报 SN（状态仍为 FORECASTED）。
func (s *Service) computeMissingSNs(tx *gorm.DB, forecastNo string) ([]string, error) {
	var sns []string
	err := tx.Model(&model.SNItem{}).
		Where("tenant_id = ? AND forecast_no = ? AND status = ?", s.Tenant, forecastNo, string(enum.SNForecasted)).
		Order("sn").Pluck("sn", &sns).Error
	return sns, err
}

// ApproveDiscReq 差异审批入参。
type ApproveDiscReq struct {
	Note      string `json:"note"`
	OldQty    int    `json:"old_qty"`
	NewQty    int    `json:"new_qty"`
	PNCode    string `json:"pn_code"`
}

// ApproveDiscrepancy 差异审批通过（DC-04 / DC-09 / DC-11 / FC-06 / SN-05）。
func (s *Service) ApproveDiscrepancy(recordNo string, req ApproveDiscReq, approver string) (*model.DiscrepancyRecord, error) {
	var rec model.DiscrepancyRecord
	err := s.Tx(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ? AND record_no = ?", s.Tenant, recordNo).First(&rec).Error; err != nil {
			return errs.NotFound("差异记录 " + recordNo)
		}
		if rec.ApprovalStatus != string(enum.ApprovalPending) {
			return errs.ErrStateNotAllowed("审批差异", rec.ApprovalStatus)
		}
		if !rec.Locked {
			return errs.Conflict("NOT_SUBMITTED", "差异尚未提交（缺佐证照片），不能审批")
		}
		now := s.now()
		updates := map[string]any{
			"approval_status": string(enum.ApprovalApproved),
			"approver":        approver, "approved_at": now, "approval_note": req.Note,
		}
		if err := tx.Model(&model.DiscrepancyRecord{}).
			Where("tenant_id = ? AND record_no = ?", s.Tenant, recordNo).Updates(updates).Error; err != nil {
			return err
		}

		if rec.Source == enum.SourceKitting {
			if err := s.applyKittingApproval(tx, &rec, req, approver); err != nil {
				return err
			}
		}
		// RECEIVING 差异：仅放行，不改预报（DC-11）
		return s.audit(tx, enum.AuditDiscApprove, "discrepancy_record", recordNo, "approval_status",
			string(enum.ApprovalPending), string(enum.ApprovalApproved), req.Note, approver, "")
	})
	if err != nil {
		return nil, err
	}
	// 审批后聚合推进
	if rec.ForecastNo != "" {
		if err := s.ReconcileRMA(rec.ForecastNo, approver); err != nil {
			return nil, err
		}
	}
	return &rec, nil
}

// applyKittingApproval Kitting 差异审批通过 → 修正预报 REVISED + 双异常记录（FC-06 / DC-05 / SN-05）。
func (s *Service) applyKittingApproval(tx *gorm.DB, rec *model.DiscrepancyRecord, req ApproveDiscReq, approver string) error {
	fo, err := s.getForecast(tx, rec.ForecastNo)
	if err != nil {
		return err
	}
	oldQty := fo.ForecastQty
	newQty := oldQty
	dt := enum.DiscrepancyType(rec.DiscrepancyType)

	switch dt {
	case enum.DiscQtyOver, enum.DiscSNNotInForecast:
		// 多收：回填预报（新增 SN 归属本 RMA），数量 +1
		if rec.SN != "" {
			pn := rec.PNCode
			if pn == "" {
				pn = fo.PNCode
			}
			var cnt int64
			tx.Model(&model.ForecastSNItem{}).
				Where("tenant_id = ? AND forecast_no = ? AND sn = ?", s.Tenant, fo.ForecastNo, rec.SN).Count(&cnt)
			if cnt == 0 {
				if err := tx.Create(&model.ForecastSNItem{
					TenantID: s.Tenant, ForecastNo: fo.ForecastNo, SN: rec.SN, RMANo: fo.RMANo, PNCode: pn,
				}).Error; err != nil {
					return err
				}
				newQty = oldQty + 1
			}
			// 多收 SN 审批通过后：先 FORECASTED 再 RECEIVED（RK-03）
			it, err := s.getSN(tx, rec.SN)
			if err != nil {
				return err
			}
			if enum.SNStatus(it.Status) == enum.SNDiscrepancy {
				if it.ForecastNo == "" {
					if err := s.setSNFields(tx, rec.SN, map[string]any{
						"forecast_no": fo.ForecastNo, "rma_no": fo.RMANo, "pn_code": pn,
					}); err != nil {
						return err
					}
					if _, err := s.transit(tx, rec.SN, enum.SNForecasted, statemachine.SNContext{AllowBackfill: true}, approver); err != nil {
						return err
					}
				}
				if _, err := s.transit(tx, rec.SN, enum.SNReceived, statemachineCtxFor(""), approver); err != nil {
					return err
				}
			}
		}
	case enum.DiscQtyShort:
		// 少收：按 missing_sn_list 逐 SN 关单（DC-09）
		missing := rec.MissingSNList
		if len(missing) == 0 {
			missing, err = s.computeMissingSNs(tx, fo.ForecastNo)
			if err != nil {
				return err
			}
		}
		for _, sn := range missing {
			if _, err := s.transit(tx, sn, enum.SNClosedShort, statemachineCtxFor(""), approver); err != nil {
				return err
			}
		}
		newQty = oldQty - len(missing)
		if newQty < 0 {
			newQty = 0
		}
		if err := tx.Model(&model.DiscrepancyRecord{}).
			Where("tenant_id = ? AND record_no = ?", s.Tenant, rec.RecordNo).
			Update("missing_sn_list", model.StrList(missing)).Error; err != nil {
			return err
		}
	default:
		// PN / RMA 不符：差异闭环，SN 回到 RECEIVED
		if rec.SN != "" {
			it, err := s.getSN(tx, rec.SN)
			if err != nil {
				return err
			}
			if enum.SNStatus(it.Status) == enum.SNDiscrepancy {
				if _, err := s.transit(tx, rec.SN, enum.SNReceived, statemachineCtxFor(""), approver); err != nil {
					return err
				}
			}
		}
	}

	// 修正预报 → REVISED（FC-06），保留修正前后（E1 无历史列，故在双异常记录中留痕）
	if err := tx.Model(&model.ForecastOrder{}).
		Where("tenant_id = ? AND forecast_no = ?", s.Tenant, fo.ForecastNo).
		Updates(map[string]any{"status": string(enum.ForecastRevised), "forecast_qty": newQty}).Error; err != nil {
		return err
	}
	// FORECAST_ANOMALY 永久记录（DC-05）
	return tx.Create(&model.DiscrepancyRecord{
		TenantID: s.Tenant, RecordNo: rec.RecordNo + "-F", Category: string(enum.AnomalyForecast),
		Source: rec.Source, DiscrepancyType: rec.DiscrepancyType,
		ForecastNo: rec.ForecastNo, BatchNo: rec.BatchNo, BoxNo: rec.BoxNo,
		SN: rec.SN, PNCode: rec.PNCode, RMANo: rec.RMANo,
		QtyDiff: newQty - oldQty, PhotoIDs: rec.PhotoIDs,
		ApprovalStatus: string(enum.ApprovalApproved), Approver: approver,
		ApprovedAt: ptrTime(s.now()),
		ApprovalNote: fmt.Sprintf("预报修正 %d → %d；%s", oldQty, newQty, req.Note),
		DiscoveredBy: rec.DiscoveredBy, DiscoveredAt: rec.DiscoveredAt, Locked: true,
	}).Error
}

// RejectDiscrepancy 差异驳回（DC-04 / RK-11）：退回并解锁允许重扫。
func (s *Service) RejectDiscrepancy(recordNo, reason, approver string) (*model.DiscrepancyRecord, error) {
	var rec model.DiscrepancyRecord
	err := s.Tx(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ? AND record_no = ?", s.Tenant, recordNo).First(&rec).Error; err != nil {
			return errs.NotFound("差异记录 " + recordNo)
		}
		if rec.ApprovalStatus != string(enum.ApprovalPending) {
			return errs.ErrStateNotAllowed("驳回差异", rec.ApprovalStatus)
		}
		now := s.now()
		if err := tx.Model(&model.DiscrepancyRecord{}).
			Where("tenant_id = ? AND record_no = ?", s.Tenant, recordNo).
			Updates(map[string]any{
				"approval_status": string(enum.ApprovalRejected),
				"reject_reason":   reason, "approver": approver, "approved_at": now, "locked": false,
			}).Error; err != nil {
			return err
		}
		// 解锁：移除箱内扫码记录，允许重扫（RK-11）
		if rec.SN != "" {
			if err := tx.Where("tenant_id = ? AND sn = ?", s.Tenant, rec.SN).
				Delete(&model.BoxSNItem{}).Error; err != nil {
				return err
			}
		}
		return s.audit(tx, enum.AuditDiscReject, "discrepancy_record", recordNo, "approval_status",
			string(enum.ApprovalPending), string(enum.ApprovalRejected), reason, approver, "")
	})
	if err != nil {
		return nil, err
	}
	rec.ApprovalStatus = string(enum.ApprovalRejected)
	rec.Locked = false
	return &rec, nil
}

// SubmitShortDiscrepancy 登记少收差异（DC-08/DC-09）：kitting 结束后实物少于预报，
// 由仓库现场登记（照片≥2：箱内物 + 箱标含数量），自动列出未到货 SN 清单并提交审核。
func (s *Service) SubmitShortDiscrepancy(forecastNo, batchNo, boxNo string, photoIDs []string, operator string) (*model.DiscrepancyRecord, error) {
	var out *model.DiscrepancyRecord
	err := s.Tx(func(tx *gorm.DB) error {
		fo, err := s.getForecast(tx, forecastNo)
		if err != nil {
			return err
		}
		missing, err := s.computeMissingSNs(tx, fo.ForecastNo)
		if err != nil {
			return err
		}
		if len(missing) == 0 {
			return errs.BadRequest("NO_MISSING_SN", "该预报无未到货 SN，无需登记少收")
		}
		need := PhotoRules(enum.DiscQtyShort)
		if len(photoIDs) < need {
			return errs.ErrPhotoRequired(need)
		}
		out, err = s.createDiscrepancy(tx, discInput{
			Source: enum.SourceKitting, Type: enum.DiscQtyShort,
			ForecastNo: fo.ForecastNo, BatchNo: batchNo, BoxNo: boxNo,
			PNCode: fo.PNCode, RMANo: fo.RMANo,
			QtyDiff: -len(missing), MissingSNList: missing, PhotoIDs: photoIDs,
		}, operator)
		if err != nil {
			return err
		}
		if err := tx.Model(&model.DiscrepancyRecord{}).
			Where("tenant_id = ? AND record_no = ?", s.Tenant, out.RecordNo).
			Update("locked", true).Error; err != nil {
			return err
		}
		out.Locked = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ListDiscrepancies 差异列表（按 RMA/订单号索引，DZ-05 三 Tab）。
func (s *Service) ListDiscrepancies(tab, forecastNo, keyword string, page, size int) (PageResult, error) {
	q := s.DB.Model(&model.DiscrepancyRecord{}).Where("tenant_id = ?", s.Tenant)
	switch tab {
	case "pending":
		q = q.Where("approval_status = ? AND category = ?", string(enum.ApprovalPending), string(enum.AnomalyArrival))
	case "done":
		q = q.Where("approval_status IN ?", []string{string(enum.ApprovalApproved), string(enum.ApprovalRejected)})
	case "anomaly":
		// 双异常查询：不过滤状态
	default:
		q = q.Where("approval_status = ?", string(enum.ApprovalPending))
	}
	if forecastNo != "" {
		q = q.Where("forecast_no = ?", forecastNo)
	}
	if keyword != "" {
		kw := "%" + keyword + "%"
		q = q.Where("rma_no LIKE ? OR record_no LIKE ? OR sn LIKE ?", kw, kw, kw)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return PageResult{}, err
	}
	var rows []model.DiscrepancyRecord
	q2, p, sz := paged(q, page, size)
	if err := q2.Order("discovered_at DESC").Find(&rows).Error; err != nil {
		return PageResult{}, err
	}
	return wrapPage(rows, total, p, sz), nil
}

// GetDiscrepancy 差异详情。
func (s *Service) GetDiscrepancy(no string) (*model.DiscrepancyRecord, error) {
	var rec model.DiscrepancyRecord
	if err := s.DB.Where("tenant_id = ? AND record_no = ?", s.Tenant, no).First(&rec).Error; err != nil {
		return nil, errs.NotFound("差异记录 " + no)
	}
	return &rec, nil
}

var _ = time.Now
