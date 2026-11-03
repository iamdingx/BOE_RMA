package service

import (
	"errors"
	"fmt"

	"gorm.io/gorm"


	"github.com/boe/rma/internal/enum"
	"github.com/boe/rma/internal/errs"
	"github.com/boe/rma/internal/model"
	"github.com/boe/rma/internal/seq"
)

// CreateOutboundReq 出库单创建入参。
type CreateOutboundReq struct {
	Type       enum.OutboundType `json:"outbound_type"`
	SNs        []string          `json:"sns"`
	RMANo      string            `json:"rma_no"`
	CIDFlag    bool              `json:"cid_flag"`
	TransferNo string            `json:"transfer_no"`
}

// CreateOutbound 创建出库单（OB-01/02/03/04/05/11）。
func (s *Service) CreateOutbound(req CreateOutboundReq, operator string) (*model.OutboundOrder, error) {
	var ob *model.OutboundOrder
	err := s.Tx(func(tx *gorm.DB) error {
		var e error
		ob, e = s.createOutboundTx(tx, req, operator)
		return e
	})
	if err != nil {
		return nil, err
	}
	return ob, nil
}

// createOutboundTx 出库单创建（事务内版本，供还货编排复用，避免嵌套事务）。
func (s *Service) createOutboundTx(tx *gorm.DB, req CreateOutboundReq, operator string) (*model.OutboundOrder, error) {
	if !req.Type.Valid() {
		return nil, errs.ErrInvalidEnum("outbound_type", string(req.Type))
	}
	if len(req.SNs) == 0 {
		return nil, errs.BadRequest("EMPTY_SNS", "出库 SN 清单不能为空")
	}
	{
		// 状态白名单校验（OB-04 / RK-07）
		for _, raw := range req.SNs {
			sn := normSN(raw)
			var it model.SNItem
			err := tx.Where("tenant_id = ? AND sn = ?", s.Tenant, sn).First(&it).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				// 可能是 buffer 备件
				var b model.BufferStock
				if e2 := tx.Where("tenant_id = ? AND sn = ?", s.Tenant, sn).First(&b).Error; e2 != nil {
					return nil, errs.NotFound("SN " + sn)
				}
				if req.Type != enum.OutShipToCustomer {
					return nil, errs.Conflict("BUFFER_SN_INVALID_TYPE", "缓冲备件仅可发回客户："+sn)
				}
				continue
			}
			if err != nil {
				return nil, err
			}
			st := enum.SNStatus(it.Status)
			switch req.Type {
			case enum.OutTransferWarehouse:
				if st != enum.SNReceived && st != enum.SNJudged && st != enum.SNRetained {
					return nil, errs.Conflict("TRANSFER_STATE_NOT_ALLOWED",
						fmt.Sprintf("SN %s 状态 %s 不可调拨（仅 RECEIVED/JUDGED/RETAINED）", sn, st))
				}
			case enum.OutScrapDisposal:
				if st != enum.SNJudged && st != enum.SNRetained {
					return nil, errs.Conflict("SCRAP_STATE_NOT_ALLOWED",
						fmt.Sprintf("SN %s 状态 %s 不可报废出库（仅 JUDGED/RETAINED）", sn, st))
				}
			case enum.OutShipToRC:
				if st != enum.SNJudged {
					return nil, errs.Conflict("SHIP_RC_STATE_NOT_ALLOWED",
						fmt.Sprintf("SN %s 状态 %s 不可发维修中心（仅 JUDGED）", sn, st))
				}
			case enum.OutShipToCustomer:
				if st != enum.SNJudged && st != enum.SNReceived {
					return nil, errs.Conflict("SHIP_CUST_STATE_NOT_ALLOWED",
						fmt.Sprintf("SN %s 状态 %s 不可发客户（仅 JUDGED）", sn, st))
				}
			}
		}
		no, err := seq.NextBizNo(tx, s.Tenant, "OUTBOUND", "OB-", 4, s.now())
		if err != nil {
			return nil, err
		}
		status := enum.OutCreated
		if req.Type == enum.OutScrapDisposal {
			status = enum.OutPendingApproval // OB-03：必须先审批
		}
		ob := &model.OutboundOrder{
			TenantID: s.Tenant, OutboundNo: no, OutboundType: string(req.Type),
			Status: string(status), RMANo: req.RMANo, CIDFlag: req.CIDFlag,
			TransferNo: req.TransferNo, Operator: operator, CreatedAt: s.now(),
		}
		if err := tx.Create(ob).Error; err != nil {
			return nil, err
		}
		for _, raw := range req.SNs {
			if err := tx.Create(&model.OutboundSNItem{
				TenantID: s.Tenant, OutboundNo: no, SN: normSN(raw),
			}).Error; err != nil {
				return nil, err
			}
			_ = tx.Model(&model.SNItem{}).
				Where("tenant_id = ? AND sn = ?", s.Tenant, normSN(raw)).
				Update("outbound_no", no).Error
		}
		return ob, nil
	}
}

// ApproveScrap 报废出库审批通过（OB-03，项目人员）。
func (s *Service) ApproveScrap(outboundNo, opinion, approver string) (*model.OutboundOrder, error) {
	var ob model.OutboundOrder
	err := s.Tx(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ? AND outbound_no = ?", s.Tenant, outboundNo).First(&ob).Error; err != nil {
			return errs.NotFound("出库单 " + outboundNo)
		}
		if ob.OutboundType != string(enum.OutScrapDisposal) {
			return errs.BadRequest("NOT_SCRAP_ORDER", "非报废出库单无需报废审批")
		}
		if ob.Status != string(enum.OutPendingApproval) {
			return errs.ErrStateNotAllowed("报废审批", ob.Status)
		}
		now := s.now()
		if err := tx.Model(&model.OutboundOrder{}).
			Where("tenant_id = ? AND outbound_no = ?", s.Tenant, outboundNo).
			Updates(map[string]any{
				"status": string(enum.OutCreated),
				"scrap_approval": model.JSONMap{
					"approver": approver, "approve_at": now.Format("2006-01-02 15:04:05"), "opinion": opinion,
				},
			}).Error; err != nil {
			return err
		}
		ob.Status = string(enum.OutCreated)
		return s.audit(tx, enum.AuditScrapApproval, "outbound_order", outboundNo, "status",
			string(enum.OutPendingApproval), string(enum.OutCreated), opinion, approver, approver)
	})
	if err != nil {
		return nil, err
	}
	return &ob, nil
}

// RejectScrap 报废出库审批驳回（L-08）：退回报废单，原片状态不变，可重提。
func (s *Service) RejectScrap(outboundNo, reason, approver string) (*model.OutboundOrder, error) {
	var ob model.OutboundOrder
	err := s.Tx(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ? AND outbound_no = ?", s.Tenant, outboundNo).First(&ob).Error; err != nil {
			return errs.NotFound("出库单 " + outboundNo)
		}
		if ob.Status != string(enum.OutPendingApproval) {
			return errs.ErrStateNotAllowed("报废驳回", ob.Status)
		}
		if err := tx.Model(&model.OutboundOrder{}).
			Where("tenant_id = ? AND outbound_no = ?", s.Tenant, outboundNo).
			Updates(map[string]any{
				"status":         string(enum.OutRejected),
				"scrap_approval": model.JSONMap{"approver": approver, "reject_reason": reason},
			}).Error; err != nil {
			return err
		}
		ob.Status = string(enum.OutRejected)
		return s.audit(tx, enum.AuditScrapApproval, "outbound_order", outboundNo, "status",
			string(enum.OutPendingApproval), string(enum.OutRejected), reason, approver, approver)
	})
	if err != nil {
		return nil, err
	}
	return &ob, nil
}

// ResubmitScrap 报废驳回重提交（L-08）。
func (s *Service) ResubmitScrap(outboundNo, operator string) (*model.OutboundOrder, error) {
	var ob model.OutboundOrder
	err := s.Tx(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ? AND outbound_no = ?", s.Tenant, outboundNo).First(&ob).Error; err != nil {
			return errs.NotFound("出库单 " + outboundNo)
		}
		if ob.Status != string(enum.OutRejected) {
			return errs.ErrStateNotAllowed("重提交报废单", ob.Status)
		}
		if err := tx.Model(&model.OutboundOrder{}).
			Where("tenant_id = ? AND outbound_no = ?", s.Tenant, outboundNo).
			Update("status", string(enum.OutPendingApproval)).Error; err != nil {
			return err
		}
		ob.Status = string(enum.OutPendingApproval)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &ob, nil
}

// ShipReq 发运入参（OB-09：物流公司 + 物流单号必填）。
type ShipReq struct {
	Carrier     string `json:"carrier"`
	TrackingNo  string `json:"tracking_no"`
	LogisticsNo string `json:"logistics_no"`
}

// ShipOutbound 执行发运（OB-02/06/09，含 buffer 件出库与原片终态裁决）。
func (s *Service) ShipOutbound(outboundNo string, req ShipReq, operator string) (*model.OutboundOrder, error) {
	if req.Carrier == "" || req.TrackingNo == "" {
		return nil, errs.BadRequest("LOGISTICS_REQUIRED", "发运必须提交物流公司(carrier)与物流单号(tracking_no)")
	}
	var ob model.OutboundOrder
	err := s.Tx(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ? AND outbound_no = ?", s.Tenant, outboundNo).First(&ob).Error; err != nil {
			return errs.NotFound("出库单 " + outboundNo)
		}
		if ob.Status != string(enum.OutCreated) {
			return errs.ErrStateNotAllowed("发运", ob.Status)
		}
		var items []model.OutboundSNItem
		if err := tx.Where("tenant_id = ? AND outbound_no = ?", s.Tenant, outboundNo).Find(&items).Error; err != nil {
			return err
		}
		obType := enum.OutboundType(ob.OutboundType)
		for _, it := range items {
			if err := s.shipOneSN(tx, it.SN, obType, outboundNo, "", operator); err != nil {
				return err
			}
		}
		now := s.now()
		if err := tx.Model(&model.OutboundOrder{}).
			Where("tenant_id = ? AND outbound_no = ?", s.Tenant, outboundNo).
			Updates(map[string]any{
				"status": string(enum.OutShipped), "carrier": req.Carrier,
				"logistics_no": firstNonEmpty(req.LogisticsNo, req.TrackingNo),
				"shipped_at": now, "outbound_at": now,
			}).Error; err != nil {
			return err
		}
		// 关联发货箱标记已发运
		if err := tx.Model(&model.ShipmentBox{}).
			Where("tenant_id = ? AND outbound_no = ?", s.Tenant, outboundNo).
			Updates(map[string]any{"carrier": req.Carrier, "tracking_no": req.TrackingNo, "shipped_at": now}).Error; err != nil {
			return err
		}
		ob.Status = string(enum.OutShipped)
		return s.audit(tx, enum.AuditOutboundShip, "outbound_order", outboundNo, "status",
			string(enum.OutCreated), string(enum.OutShipped), req.Carrier+" / "+req.TrackingNo, operator, "")
	})
	if err != nil {
		return nil, err
	}
	if ob.RMANo != "" {
		if err := s.ReconcileRMA(s.forecastNoByRMA(ob.RMANo), operator); err != nil {
			return nil, err
		}
	}
	return &ob, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// shipOneSN 单个 SN 出库裁决：buffer 件走库存状态，退货原片走 SN 状态机。
func (s *Service) shipOneSN(tx *gorm.DB, sn string, obType enum.OutboundType, outboundNo, boxNo, operator string) error {
	// 解析该 SN 所在发货箱（若有）
	if boxNo == "" {
		var sbs model.ShipmentBoxSN
		if err := tx.Where("tenant_id = ? AND sn = ?", s.Tenant, sn).First(&sbs).Error; err == nil {
			var sb model.ShipmentBox
			if err := tx.Where("tenant_id = ? AND shipment_box_no = ? AND outbound_no = ?",
				s.Tenant, sbs.ShipmentBoxNo, outboundNo).First(&sb).Error; err == nil {
				boxNo = sb.ShipmentBoxNo
			}
		}
	}
	var buf model.BufferStock
	bufErr := tx.Where("tenant_id = ? AND sn = ?", s.Tenant, sn).First(&buf).Error
	var it model.SNItem
	itErr := tx.Where("tenant_id = ? AND sn = ?", s.Tenant, sn).First(&it).Error

	isBuffer := bufErr == nil && (itErr != nil || it.Status == string(enum.SNCompleted) || it.Status == string(enum.SNDisposed))
	if isBuffer {
		if buf.StockStatus == string(enum.BufferOut) {
			return errs.Conflict("BUFFER_ALREADY_OUT", "备件 "+sn+" 已出库")
		}
		if err := tx.Model(&model.BufferStock{}).
			Where("tenant_id = ? AND sn = ?", s.Tenant, sn).
			Updates(map[string]any{"stock_status": string(enum.BufferOut)}).Error; err != nil {
			return err
		}
		// 原片裁决：BUFFER 还货中"原片留库待处置"的，随备件发运置 RETAINED
		var rr model.ReturnRecord
		if err := tx.Where("tenant_id = ? AND returned_sn = ?", s.Tenant, sn).First(&rr).Error; err == nil {
			if enum.OriginalDisposition(rr.OriginalDisposition) == enum.OrigRetained {
				var orig model.SNItem
				if e := tx.Where("tenant_id = ? AND sn = ?", s.Tenant, rr.OriginalSN).First(&orig).Error; e == nil {
					if enum.SNStatus(orig.Status) == enum.SNJudged {
						if _, e2 := s.transit(tx, rr.OriginalSN, enum.SNRetained, statemachineCtxFor(""), operator); e2 != nil {
							return e2
						}
					}
				}
			}
		}
		// 关联还货记录：标记已还货
		return tx.Model(&model.ReturnRecord{}).
			Where("tenant_id = ? AND returned_sn = ? AND status <> ?", s.Tenant, sn, "SHIPPED").
			Updates(map[string]any{
				"status": "SHIPPED", "returned_at": s.now(),
				"outbound_no": outboundNo, "shipment_box_no": boxNo,
			}).Error
	}
	if itErr != nil {
		return errs.NotFound("SN " + sn)
	}
	// 退货原片：JUDGED/RECEIVED/RETAINED → OUTBOUND → 终态
	ctx := statemachineCtxFor(obType)
	if _, err := s.transit(tx, sn, enum.SNOutbound, ctx, operator); err != nil {
		return err
	}
	switch obType {
	case enum.OutShipToCustomer, enum.OutTransferWarehouse:
		if _, err := s.transit(tx, sn, enum.SNCompleted, ctx, operator); err != nil {
			return err
		}
	case enum.OutShipToRC, enum.OutScrapDisposal:
		if _, err := s.transit(tx, sn, enum.SNDisposed, ctx, operator); err != nil {
			return err
		}
	}
	// 原片发客户（原片修复/CID）：还货记录置已还货
	if obType == enum.OutShipToCustomer {
		if err := tx.Model(&model.ReturnRecord{}).
			Where("tenant_id = ? AND returned_sn = ? AND status <> ?", s.Tenant, sn, "SHIPPED").
			Updates(map[string]any{
				"status": "SHIPPED", "returned_at": s.now(),
				"outbound_no": outboundNo, "shipment_box_no": boxNo,
			}).Error; err != nil {
			return err
		}
	}
	return nil
}

// ConfirmCount 调拨清点确认（OB-04 / M-10）：CREATED → 清点确认 → SHIPPED，SN 终态 COMPLETED。
func (s *Service) ConfirmCount(outboundNo, operator string) (*model.OutboundOrder, error) {
	var ob model.OutboundOrder
	err := s.Tx(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ? AND outbound_no = ?", s.Tenant, outboundNo).First(&ob).Error; err != nil {
			return errs.NotFound("出库单 " + outboundNo)
		}
		if ob.OutboundType != string(enum.OutTransferWarehouse) {
			return errs.BadRequest("NOT_TRANSFER_ORDER", "非调拨单无需清点确认")
		}
		if ob.Status != string(enum.OutCreated) {
			return errs.ErrStateNotAllowed("清点确认", ob.Status)
		}
		var items []model.OutboundSNItem
		if err := tx.Where("tenant_id = ? AND outbound_no = ?", s.Tenant, outboundNo).Find(&items).Error; err != nil {
			return err
		}
		for _, it := range items {
			if err := s.shipOneSN(tx, it.SN, enum.OutTransferWarehouse, outboundNo, "", operator); err != nil {
				return err
			}
		}
		now := s.now()
		if err := tx.Model(&model.OutboundOrder{}).
			Where("tenant_id = ? AND outbound_no = ?", s.Tenant, outboundNo).
			Updates(map[string]any{
				"status": string(enum.OutShipped), "count_confirmed_by": operator,
				"count_confirmed_at": now, "outbound_at": now,
			}).Error; err != nil {
			return err
		}
		ob.Status = string(enum.OutShipped)
		return s.audit(tx, enum.AuditControlledCorrection, "outbound_order", outboundNo, "status",
			string(enum.OutCreated), string(enum.OutShipped), "调拨清点确认", operator, "")
	})
	if err != nil {
		return nil, err
	}
	if ob.RMANo != "" {
		if err := s.ReconcileRMA(s.forecastNoByRMA(ob.RMANo), operator); err != nil {
			return nil, err
		}
	}
	return &ob, nil
}

// forecastNoByRMA RMA 号 → 预报单号。
func (s *Service) forecastNoByRMA(rmaNo string) string {
	if rmaNo == "" {
		return ""
	}
	var fo model.ForecastOrder
	if err := s.DB.Where("tenant_id = ? AND rma_no = ?", s.Tenant, rmaNo).First(&fo).Error; err != nil {
		return ""
	}
	return fo.ForecastNo
}

// ListOutbounds 出库单列表（DZ-06 按类型 4 Tab；列表不铺 SN）。
func (s *Service) ListOutbounds(obType, status, keyword string, page, size int) (PageResult, error) {
	q := s.DB.Model(&model.OutboundOrder{}).Where("tenant_id = ?", s.Tenant)
	if obType != "" {
		q = q.Where("outbound_type = ?", obType)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if keyword != "" {
		kw := "%" + keyword + "%"
		q = q.Where("outbound_no LIKE ? OR rma_no LIKE ?", kw, kw)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return PageResult{}, err
	}
	var rows []model.OutboundOrder
	q2, p, sz := paged(q, page, size)
	if err := q2.Order("created_at DESC").Find(&rows).Error; err != nil {
		return PageResult{}, err
	}
	// 附 SN 数
	type row struct {
		model.OutboundOrder
		SNCount int64 `json:"sn_count"`
	}
	out := make([]row, 0, len(rows))
	for _, r := range rows {
		var c int64
		s.DB.Model(&model.OutboundSNItem{}).Where("tenant_id = ? AND outbound_no = ?", s.Tenant, r.OutboundNo).Count(&c)
		out = append(out, row{OutboundOrder: r, SNCount: c})
	}
	return wrapPage(out, total, p, sz), nil
}

// OutboundDetail 出库单详情（含 SN 明细，点击进详情才铺 SN）。
func (s *Service) OutboundDetail(no string) (map[string]any, error) {
	var ob model.OutboundOrder
	if err := s.DB.Where("tenant_id = ? AND outbound_no = ?", s.Tenant, no).First(&ob).Error; err != nil {
		return nil, errs.NotFound("出库单 " + no)
	}
	var sns []model.OutboundSNItem
	if err := s.DB.Where("tenant_id = ? AND outbound_no = ?", s.Tenant, no).Find(&sns).Error; err != nil {
		return nil, err
	}
	var boxes []model.ShipmentBox
	if err := s.DB.Where("tenant_id = ? AND outbound_no = ?", s.Tenant, no).Find(&boxes).Error; err != nil {
		return nil, err
	}
	return map[string]any{"outbound": ob, "sn_items": sns, "shipment_boxes": boxes}, nil
}
