package service

import (
	"gorm.io/gorm"

	"github.com/boe/rma/internal/enum"
	"github.com/boe/rma/internal/errs"
	"github.com/boe/rma/internal/model"
	"github.com/boe/rma/internal/seq"
	"github.com/boe/rma/internal/statemachine"
)

// CreateBatchReq 物流允收入参（V1.22：只确认外箱标识数量 + POD，不开箱）。
type CreateBatchReq struct {
	ForecastNo    string   `json:"forecast_no"`
	LabelQtyTotal int      `json:"label_qty_total"`
	PODPhotoIDs   []string `json:"pod_photo_ids"`
	Remark        string   `json:"remark"`
}

// CreateBatch 创建允收批次（RC-01/02/03，DC-11 RECEIVING 差异）。
func (s *Service) CreateBatch(req CreateBatchReq, operator string) (*model.ReceivingBatch, error) {
	if len(req.PODPhotoIDs) == 0 {
		return nil, errs.BadRequest("POD_REQUIRED", "物流允收必须上传 POD 照片（至少 1 张）")
	}
	if req.LabelQtyTotal < 0 {
		return nil, errs.BadRequest("INVALID_QTY", "外箱标识数量不能为负")
	}
	var batch *model.ReceivingBatch
	err := s.Tx(func(tx *gorm.DB) error {
		fo, err := s.getForecast(tx, req.ForecastNo)
		if err != nil {
			return err
		}
		no, err := seq.NextBizNo(tx, s.Tenant, "BATCH", "RCV", 3, s.now())
		if err != nil {
			return err
		}
		diff := req.LabelQtyTotal - fo.ForecastQty
		batch = &model.ReceivingBatch{
			TenantID: s.Tenant, BatchNo: no, ForecastNo: fo.ForecastNo,
			PodPhotoIDs: req.PODPhotoIDs, LabelQtyTotal: req.LabelQtyTotal, QtyDiff: diff,
			Status: "RECEIVED", ReceivedBy: operator, ReceivedAt: s.now(),
		}
		if err := tx.Create(batch).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.ForecastOrder{}).
			Where("tenant_id = ? AND forecast_no = ?", s.Tenant, fo.ForecastNo).
			Update("label_qty_total", req.LabelQtyTotal).Error; err != nil {
			return err
		}
		// 已允收数据锁定（RC-03）
		if err := s.audit(tx, enum.AuditControlledCorrection, "receiving_batch", no, "status",
			"", "RECEIVED", "物流允收："+req.Remark, operator, ""); err != nil {
			return err
		}
		// RMA OPEN → RECEIVING（首批到货允收）
		if enum.RMAStatus(fo.RMAStatus) == enum.RMAOpen {
			if err := s.advanceRMA(tx, fo, enum.RMAReceiving, operator); err != nil {
				return err
			}
		}
		// RECEIVING 差异：仅放行、不改预报（DC-11 / R-004）
		if diff != 0 {
			dt := enum.DiscQtyOver
			if diff < 0 {
				dt = enum.DiscQtyShort
			}
			dno, err := seq.NextBizNo(tx, s.Tenant, "DISCREPANCY", "D", 4, s.now())
			if err != nil {
				return err
			}
			rec := &model.DiscrepancyRecord{
				TenantID: s.Tenant, RecordNo: dno, Category: string(enum.AnomalyArrival),
				Source: enum.SourceReceiving, DiscrepancyType: string(dt),
				ForecastNo: fo.ForecastNo, BatchNo: no, RMANo: fo.RMANo,
				QtyDiff: diff, PhotoIDs: req.PODPhotoIDs, ApprovalStatus: string(enum.ApprovalPending),
				DiscoveredBy: operator, DiscoveredAt: s.now(), Locked: true,
			}
			if err := tx.Create(rec).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return batch, nil
}

// ListBatches 允收批次列表（RC-08：预报量/外箱标识数/差异/POD/状态）。
func (s *Service) ListBatches(forecastNo string, page, size int) (PageResult, error) {
	q := s.DB.Model(&model.ReceivingBatch{}).Where("tenant_id = ?", s.Tenant)
	if forecastNo != "" {
		q = q.Where("forecast_no = ?", forecastNo)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return PageResult{}, err
	}
	var rows []model.ReceivingBatch
	q2, p, sz := paged(q, page, size)
	if err := q2.Order("received_at DESC").Find(&rows).Error; err != nil {
		return PageResult{}, err
	}
	return wrapPage(rows, total, p, sz), nil
}

// advanceRMA 推进 RMA 状态（唯一入口，含状态机校验）。
func (s *Service) advanceRMA(tx *gorm.DB, fo *model.ForecastOrder, to enum.RMAStatus, operator string) error {
	from := enum.RMAStatus(fo.RMAStatus)
	if from == to {
		return nil
	}
	if err := statemachine.CanRMATransit(from, to); err != nil {
		return err
	}
	updates := map[string]any{"rma_status": string(to)}
	// 到齐判定成立时记录 TAT 起算点（R-008）
	if to == enum.RMAProcessing && fo.FullReceivedAt == nil {
		now := s.now()
		updates["full_received_at"] = now
		fo.FullReceivedAt = &now
	}
	// RMA 自动关闭：closed_at 与"完整发货完成"同一时点（C-05）
	if to == enum.RMAClosed {
		now := s.now()
		updates["closed_at"] = now
		fo.ClosedAt = &now
	}
	if err := tx.Model(&model.ForecastOrder{}).
		Where("tenant_id = ? AND forecast_no = ?", s.Tenant, fo.ForecastNo).
		Updates(updates).Error; err != nil {
		return err
	}
	fo.RMAStatus = string(to)
	if err := s.audit(tx, enum.AuditStatusTransition, "forecast_order", fo.ForecastNo, "rma_status",
		string(from), string(to), "RMA 聚合推进", operator, ""); err != nil {
		return err
	}
	return nil
}
