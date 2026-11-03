package service

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/boe/rma/internal/enum"
	"github.com/boe/rma/internal/errs"
	"github.com/boe/rma/internal/model"
	"github.com/boe/rma/internal/seq"
)

// JudgeReq 质检判定入参。
type JudgeReq struct {
	SN                   string        `json:"sn"`
	Judgment             enum.Judgment `json:"judgment_result"`
	CIDFlag              bool          `json:"cid_flag"`
	DamagePhotoIDs       []string      `json:"damage_photo_ids"`
	CustomerConfirmation string        `json:"customer_confirmation"`
	Note                 string        `json:"note"`
}

// Judge 质检判定录入（IN-01/02/03/04，三段式自动带出还货方式 RT-01）。
func (s *Service) Judge(req JudgeReq, operator string) (*model.SNItem, error) {
	out, err := s.JudgeBatch([]JudgeReq{req}, operator)
	if err != nil {
		return nil, err
	}
	return out[0], nil
}

// JudgeBatch 批量判定（P-05 支持多选一次提交）。
func (s *Service) JudgeBatch(reqs []JudgeReq, operator string) ([]*model.SNItem, error) {
	if len(reqs) == 0 {
		return nil, errs.BadRequest("EMPTY_BATCH", "未选择待判定 SN")
	}
	var out []*model.SNItem
	err := s.Tx(func(tx *gorm.DB) error {
		for _, req := range reqs {
			sn := normSN(req.SN)
			it, err := s.getSN(tx, sn)
			if err != nil {
				return err
			}
			// IN-03 / IN-04：仅 RECEIVED 可质检
			if enum.SNStatus(it.Status) != enum.SNReceived {
				return errs.ErrStateNotAllowed("质检判定", it.Status)
			}
			if !req.Judgment.Valid() {
				return errs.ErrInvalidEnum("judgment_result", string(req.Judgment))
			}
			// IN-02：CID 必须责任标记 + 损坏照片 + 项目人员确认
			if req.Judgment == enum.JudgeCID {
				if !req.CIDFlag {
					return errs.BadRequest("CID_FLAG_REQUIRED", "CID 判定必须标记责任（客户责）")
				}
				if len(req.DamagePhotoIDs) == 0 {
					return errs.ErrPhotoRequired(1)
				}
				if req.CustomerConfirmation == "" {
					return errs.BadRequest("CID_CONFIRM_REQUIRED", "CID 判定必须录入项目人员确认记录")
				}
			}
			// RECEIVED → INSPECTING → JUDGED
			if _, err := s.transit(tx, sn, enum.SNInspecting, statemachineCtxFor(""), operator); err != nil {
				return err
			}
			if _, err := s.transit(tx, sn, enum.SNJudged, statemachineCtxFor(""), operator); err != nil {
				return err
			}
			method := enum.DeriveReturnMethod(req.Judgment)
			if err := s.setSNFields(tx, sn, map[string]any{
				"judgment_result": string(req.Judgment),
				"return_method":   string(method),
				"cid_flag":        req.CIDFlag,
			}); err != nil {
				return err
			}
			rid, err := seq.NextBizNo(tx, s.Tenant, "INSPECTION", "IN", 4, s.now())
			if err != nil {
				return err
			}
			if err := tx.Create(&model.InspectionDisposition{
				TenantID: s.Tenant, RecordID: rid, SN: sn, JudgmentResult: string(req.Judgment),
				CIDFlag: req.CIDFlag, DamagePhotoIDs: req.DamagePhotoIDs,
				CustomerConfirmation: req.CustomerConfirmation,
				Inspector:            operator, InspectionAt: s.now(),
			}).Error; err != nil {
				return err
			}
			if err := s.audit(tx, enum.AuditInspectionJudge, "sn_item", sn, "judgment_result",
				"", string(req.Judgment), fmt.Sprintf("判定=%s，还货方式自动带出=%s；%s", req.Judgment, method, req.Note),
				operator, ""); err != nil {
				return err
			}
			it.Status = string(enum.SNJudged)
			it.JudgmentResult = string(req.Judgment)
			it.ReturnMethod = string(method)
			out = append(out, it)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// 判定完成后聚合推进（PROCESSING → PENDING_RETURN）
	for _, it := range out {
		if it.ForecastNo != "" {
			if err := s.ReconcileRMA(it.ForecastNo, operator); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

// ListInspection 待质检/已判定列表（仅 RECEIVED 可操作）。
func (s *Service) ListInspection(status, forecastNo string, page, size int) (PageResult, error) {
	q := s.DB.Model(&model.SNItem{}).Where("tenant_id = ?", s.Tenant)
	if status != "" {
		q = q.Where("status = ?", status)
	} else {
		q = q.Where("status = ?", string(enum.SNReceived))
	}
	if forecastNo != "" {
		q = q.Where("forecast_no = ?", forecastNo)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return PageResult{}, err
	}
	var rows []model.SNItem
	q2, p, sz := paged(q, page, size)
	if err := q2.Order("received_at DESC").Find(&rows).Error; err != nil {
		return PageResult{}, err
	}
	return wrapPage(rows, total, p, sz), nil
}
