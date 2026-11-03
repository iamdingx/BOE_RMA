package service

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/boe/rma/internal/enum"
	"github.com/boe/rma/internal/errs"
	"github.com/boe/rma/internal/model"
	"github.com/boe/rma/internal/seq"
	"github.com/boe/rma/internal/statemachine"
)

// ForecastSN 预报 SN 明细入参（支持一 RMA 多 PN，L-06）。
type ForecastSN struct {
	SN     string `json:"sn"`
	PNCode string `json:"pn_code"`
}

// CreateForecastReq 预报录入入参。
type CreateForecastReq struct {
	RMANo                  string       `json:"rma_no"`
	PNCode                 string       `json:"pn_code"`
	ModelCode              string       `json:"model_code"`
	Customer               string       `json:"customer"`
	ForecastQty            int          `json:"forecast_qty"`
	Items                  []ForecastSN `json:"items"`
	SNList                 []string     `json:"sn_list"`
	ConfirmSecondaryReturn bool         `json:"confirm_secondary_return"`
	// LateReceive 迟到货重收（DC-10）：允许 CLOSED_SHORT 的 SN 新建收货记录重新接收。
	LateReceive bool `json:"late_receive"`
}

// CreateForecast 预报录入（FC-01/02/03/04/05/07，SN-02）。
func (s *Service) CreateForecast(req CreateForecastReq, operator string) (*model.ForecastOrder, error) {
	if req.RMANo == "" || req.Customer == "" {
		return nil, errs.BadRequest("INVALID_PARAM", "RMA 号与客户必填")
	}
	items, err := normalizeForecastItems(req)
	if err != nil {
		return nil, err
	}
	qty := req.ForecastQty
	if qty == 0 {
		qty = len(items)
	}
	if qty != len(items) {
		return nil, errs.ErrQtyMismatch(qty, len(items))
	}
	if qty <= 0 {
		return nil, errs.BadRequest("EMPTY_SN_LIST", "SN 清单不能为空")
	}

	var fo *model.ForecastOrder
	err = s.Tx(func(tx *gorm.DB) error {
		// FC-07 料号必须在主数据
		for _, it := range items {
			var pn model.PartNumber
			if err := tx.Where("tenant_id = ? AND pn_code = ?", s.Tenant, it.PNCode).First(&pn).Error; err != nil {
				return errs.BadRequest("PN_NOT_FOUND", "料号 "+it.PNCode+" 不在主数据中，请先维护主数据")
			}
		}
		// RMA 唯一（1 RMA = 1 预报单）
		var dup int64
		if err := tx.Model(&model.ForecastOrder{}).
			Where("tenant_id = ? AND rma_no = ?", s.Tenant, req.RMANo).Count(&dup).Error; err != nil {
			return err
		}
		if dup > 0 {
			return errs.Conflict("RMA_EXISTS", "RMA 号 "+req.RMANo+" 已存在预报单")
		}
		// SN 清单内部重复
		seen := map[string]bool{}
		for _, it := range items {
			if seen[it.SN] {
				return errs.BadRequest("DUPLICATE_SN", "SN 清单存在重复："+it.SN)
			}
			seen[it.SN] = true
		}
		// FC-04 / SN-02 已有 SN：仅 COMPLETED 可二次返修
		for _, it := range items {
			var exist model.SNItem
			err := tx.Where("tenant_id = ? AND sn = ?", s.Tenant, it.SN).First(&exist).Error
			if err != nil && err != gorm.ErrRecordNotFound {
				return err
			}
			if err == gorm.ErrRecordNotFound {
				continue
			}
			if enum.SNStatus(exist.Status) != enum.SNCompleted {
				// DC-10：CLOSED_SHORT 迟到货，允许走"新建收货记录重收"（需显式 late_receive）
				if !(req.LateReceive && enum.SNStatus(exist.Status) == enum.SNClosedShort) {
					return errs.Conflict("SECONDARY_RETURN_BLOCKED",
						fmt.Sprintf("SN %s 当前状态 %s，仅 COMPLETED 允许二次返修", it.SN, exist.Status))
				}
			}
			if !req.ConfirmSecondaryReturn && enum.SNStatus(exist.Status) == enum.SNCompleted {
				return errs.Conflict("SECONDARY_RETURN_CONFIRM_REQUIRED",
					"SN "+it.SN+" 为二次返修，请确认后重试（confirm_secondary_return=true）")
			}
		}

		no, err := seq.NextBizNo(tx, s.Tenant, "FORECAST", "F", 4, s.now())
		if err != nil {
			return err
		}
		fo = &model.ForecastOrder{
			TenantID: s.Tenant, ForecastNo: no, RMANo: req.RMANo,
			PNCode: items[0].PNCode, ModelCode: req.ModelCode,
			ForecastQty: qty, Customer: req.Customer,
			Status: string(enum.ForecastEntered), RMAStatus: string(enum.RMAOpen),
			CreatedBy: operator, CreatedAt: s.now(),
		}
		if err := tx.Create(fo).Error; err != nil {
			return err
		}
		for _, it := range items {
			if err := tx.Create(&model.ForecastSNItem{
				TenantID: s.Tenant, ForecastNo: no, SN: it.SN, RMANo: req.RMANo, PNCode: it.PNCode,
			}).Error; err != nil {
				return err
			}
			var exist model.SNItem
			err := tx.Where("tenant_id = ? AND sn = ?", s.Tenant, it.SN).First(&exist).Error
			if err == gorm.ErrRecordNotFound {
				if err := tx.Create(&model.SNItem{
					TenantID: s.Tenant, SN: it.SN, Status: string(enum.SNForecasted),
					RMANo: req.RMANo, PNCode: it.PNCode, ForecastNo: no, Owner: operator,
				}).Error; err != nil {
					return err
				}
				continue
			}
			if err != nil {
				return err
			}
			// 归档上一轮 → sn_item_round（SN-02）
			roundNo := exist.ReturnCount + 1
			if err := tx.Create(&model.SNItemRound{
				TenantID: s.Tenant, RoundID: fmt.Sprintf("%s-R%d", it.SN, roundNo),
				SN: it.SN, RoundNo: roundNo, RMANo: exist.RMANo, PNCode: exist.PNCode,
				ForecastNo: exist.ForecastNo, FromStatus: exist.Status, ToStatus: string(enum.SNCompleted),
				StartedAt: exist.ReceivedAt, EndedAt: exist.CompletedAt,
				ReceivedAt: exist.ReceivedAt, JudgedAt: exist.JudgedAt, OutboundAt: exist.OutboundAt,
				JudgmentResult: exist.JudgmentResult,
			}).Error; err != nil {
				return err
			}
			// 重置进入新一轮 FORECASTED
			if _, err := s.transit(tx, it.SN, enum.SNForecasted, statemachineCtxFor(""), operator); err != nil {
				return err
			}
			if err := s.setSNFields(tx, it.SN, map[string]any{
				"rma_no": req.RMANo, "pn_code": it.PNCode, "forecast_no": no,
				"batch_no": nil, "box_no": nil, "judgment_result": nil, "return_method": nil,
				"outbound_no": nil, "cid_flag": false, "locked": false,
				"received_at": nil, "inspected_at": nil, "judged_at": nil,
				"outbound_at": nil, "completed_at": nil,
				"return_count": roundNo, "secondary_return_flag": true,
			}); err != nil {
				return err
			}
		}
		return s.audit(tx, enum.AuditControlledCorrection, "forecast_order", no, "", "", "", "预报录入", operator, "")
	})
	if err != nil {
		return nil, err
	}
	return fo, nil
}

// normalizeForecastItems 归并 Items / SNList 两种入参。
func normalizeForecastItems(req CreateForecastReq) ([]ForecastSN, error) {
	out := make([]ForecastSN, 0, len(req.Items)+len(req.SNList))
	if len(req.Items) > 0 {
		for _, it := range req.Items {
			sn := normSN(it.SN)
			if sn == "" {
				return nil, errs.BadRequest("INVALID_SN", "SN 不能为空")
			}
			pn := trimAll(it.PNCode)
			if pn == "" {
				pn = trimAll(req.PNCode)
			}
			if pn == "" {
				return nil, errs.BadRequest("INVALID_PN", "料号不能为空")
			}
			out = append(out, ForecastSN{SN: sn, PNCode: pn})
		}
		return out, nil
	}
	pn := trimAll(req.PNCode)
	if pn == "" {
		return nil, errs.BadRequest("INVALID_PN", "料号不能为空")
	}
	for _, sn := range req.SNList {
		v := normSN(sn)
		if v == "" {
			continue
		}
		out = append(out, ForecastSN{SN: v, PNCode: pn})
	}
	return out, nil
}

// ListForecast 预报/RMA 列表（分页 10/20/50/200）。
func (s *Service) ListForecast(status, customer, keyword string, page, size int) (PageResult, error) {
	q := s.DB.Model(&model.ForecastOrder{}).Where("tenant_id = ?", s.Tenant)
	if status != "" {
		q = q.Where("rma_status = ?", status)
	}
	if customer != "" {
		q = q.Where("customer = ?", customer)
	}
	if keyword != "" {
		kw := "%" + keyword + "%"
		q = q.Where("rma_no LIKE ? OR forecast_no LIKE ?", kw, kw)
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
	return wrapPage(rows, total, p, sz), nil
}

// GetForecast 预报详情（含 SN 明细与还货进度）。
func (s *Service) GetForecast(forecastNo string) (map[string]any, error) {
	fo, err := s.getForecast(s.DB, forecastNo)
	if err != nil {
		return nil, err
	}
	var items []model.ForecastSNItem
	if err := s.DB.Where("tenant_id = ? AND forecast_no = ?", s.Tenant, forecastNo).Find(&items).Error; err != nil {
		return nil, err
	}
	var sns []model.SNItem
	if err := s.DB.Where("tenant_id = ? AND forecast_no = ?", s.Tenant, forecastNo).Find(&sns).Error; err != nil {
		return nil, err
	}
	return map[string]any{"forecast": fo, "sn_items": items, "sn_status": sns}, nil
}

// statemachineCtxFor 构造跃迁上下文。
func statemachineCtxFor(t enum.OutboundType) statemachine.SNContext {
	return statemachine.SNContext{OutboundType: t}
}

// Excel SN 导入：见 excel.go。
