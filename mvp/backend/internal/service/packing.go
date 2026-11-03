package service

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/boe/rma/internal/enum"
	"github.com/boe/rma/internal/errs"
	"github.com/boe/rma/internal/model"
	"github.com/boe/rma/internal/seq"
)

// CreateShipBoxReq 发货箱创建入参。
type CreateShipBoxReq struct {
	OutboundNo string   `json:"outbound_no"`
	RMANos     []string `json:"rma_nos"`
}

// CreateShipmentBox 新建发货箱（OB-07/10）：OB+YYMMDD+3位，独立序列。
func (s *Service) CreateShipmentBox(req CreateShipBoxReq, operator string) (*model.ShipmentBox, error) {
	var box *model.ShipmentBox
	err := s.Tx(func(tx *gorm.DB) error {
		var ob model.OutboundOrder
		if err := tx.Where("tenant_id = ? AND outbound_no = ?", s.Tenant, req.OutboundNo).First(&ob).Error; err != nil {
			return errs.NotFound("出库单 " + req.OutboundNo)
		}
		if ob.Status != string(enum.OutCreated) {
			return errs.ErrStateNotAllowed("新建发货箱", ob.Status)
		}
		no, err := seq.NextBoxNo(tx, s.Tenant, model.SeqShip, s.now())
		if err != nil {
			return err
		}
		rmas := req.RMANos
		if len(rmas) == 0 && ob.RMANo != "" {
			rmas = []string{ob.RMANo}
		}
		box = &model.ShipmentBox{
			TenantID: s.Tenant, ShipmentBoxNo: no, RMANos: rmas,
			OutboundNo: req.OutboundNo, PackedBy: operator, PackedAt: s.now(),
		}
		return tx.Create(box).Error
	})
	if err != nil {
		return nil, err
	}
	return box, nil
}

// AddShipmentItem 扫 SN 入发货箱（OB-07）。
func (s *Service) AddShipmentItem(boxNo, sn, operator string) (*model.ShipmentBox, error) {
	var box model.ShipmentBox
	err := s.Tx(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ? AND shipment_box_no = ?", s.Tenant, boxNo).First(&box).Error; err != nil {
			return errs.NotFound("发货箱 " + boxNo)
		}
		if box.Sealed {
			return errs.Conflict("SHIP_BOX_SEALED", "发货箱已封箱")
		}
		sn = normSN(sn)
		// 必须属于该出库单
		var cnt int64
		if err := tx.Model(&model.OutboundSNItem{}).
			Where("tenant_id = ? AND outbound_no = ? AND sn = ?", s.Tenant, box.OutboundNo, sn).Count(&cnt).Error; err != nil {
			return err
		}
		if cnt == 0 {
			return errs.BadRequest("SN_NOT_IN_OUTBOUND", "SN "+sn+" 不在该出库单明细中")
		}
		var dup int64
		if err := tx.Model(&model.ShipmentBoxSN{}).
			Where("tenant_id = ? AND sn = ?", s.Tenant, sn).Count(&dup).Error; err != nil {
			return err
		}
		if dup > 0 {
			return errs.Conflict("SN_DUPLICATED", "SN "+sn+" 已装入其他发货箱")
		}
		if err := tx.Create(&model.ShipmentBoxSN{
			TenantID: s.Tenant, ShipmentBoxNo: boxNo, SN: sn,
		}).Error; err != nil {
			return err
		}
		var items []model.ShipmentBoxSN
		if err := tx.Where("tenant_id = ? AND shipment_box_no = ?", s.Tenant, boxNo).Find(&items).Error; err != nil {
			return err
		}
		sns := make([]string, 0, len(items))
		for _, it := range items {
			sns = append(sns, it.SN)
		}
		return tx.Model(&model.ShipmentBox{}).
			Where("tenant_id = ? AND shipment_box_no = ?", s.Tenant, boxNo).
			Update("sn_list", model.StrList(sns)).Error
	})
	if err != nil {
		return nil, err
	}
	return s.GetShipmentBox(boxNo)
}

// SealShipBoxReq 封箱入参（重量/材积必填，OB-07/08）。
type SealShipBoxReq struct {
	Weight float64 `json:"weight"`
	Volume float64 `json:"volume"`
}

// SealShipmentBox 封箱并生成箱标打印记录。
func (s *Service) SealShipmentBox(boxNo string, req SealShipBoxReq, operator string) (*model.ShipmentBox, error) {
	err := s.Tx(func(tx *gorm.DB) error {
		var box model.ShipmentBox
		if err := tx.Where("tenant_id = ? AND shipment_box_no = ?", s.Tenant, boxNo).First(&box).Error; err != nil {
			return errs.NotFound("发货箱 " + boxNo)
		}
		if box.Sealed {
			return errs.Conflict("SHIP_BOX_SEALED", "发货箱已封箱")
		}
		if len(box.SNList) == 0 {
			return errs.BadRequest("EMPTY_BOX", "空箱不能封箱")
		}
		if req.Weight <= 0 || req.Volume <= 0 {
			return errs.BadRequest("WEIGHT_VOLUME_REQUIRED", "重量与材积必填且需大于 0")
		}
		now := s.now()
		if err := tx.Model(&model.ShipmentBox{}).
			Where("tenant_id = ? AND shipment_box_no = ?", s.Tenant, boxNo).
			Updates(map[string]any{
				"sealed": true, "weight": req.Weight, "volume": req.Volume,
				"label_print_log": model.JSONMap{
					"printed_at": now.Format("2006-01-02 15:04:05"), "operator": operator,
				},
			}).Error; err != nil {
			return err
		}
		return s.audit(tx, enum.AuditBoxSeal, "shipment_box", boxNo, "sealed",
			"false", "true", fmt.Sprintf("重量=%.3fkg 材积=%.4f", req.Weight, req.Volume), operator, "")
	})
	if err != nil {
		return nil, err
	}
	return s.GetShipmentBox(boxNo)
}

// GetShipmentBox 发货箱详情。
func (s *Service) GetShipmentBox(boxNo string) (*model.ShipmentBox, error) {
	var box model.ShipmentBox
	if err := s.DB.Where("tenant_id = ? AND shipment_box_no = ?", s.Tenant, boxNo).First(&box).Error; err != nil {
		return nil, errs.NotFound("发货箱 " + boxNo)
	}
	return &box, nil
}

// ListShipmentBoxes 发货箱列表。
func (s *Service) ListShipmentBoxes(outboundNo string, page, size int) (PageResult, error) {
	q := s.DB.Model(&model.ShipmentBox{}).Where("tenant_id = ?", s.Tenant)
	if outboundNo != "" {
		q = q.Where("outbound_no = ?", outboundNo)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return PageResult{}, err
	}
	var rows []model.ShipmentBox
	q2, p, sz := paged(q, page, size)
	if err := q2.Order("packed_at DESC").Find(&rows).Error; err != nil {
		return PageResult{}, err
	}
	return wrapPage(rows, total, p, sz), nil
}

// ShipmentLabelHTML 发货箱标（OB-08）：还货RMA+箱号+SN+重量+材积，浏览器打印。
func (s *Service) ShipmentLabelHTML(boxNo string) (string, error) {
	box, err := s.GetShipmentBox(boxNo)
	if err != nil {
		return "", err
	}
	w, v := 0.0, 0.0
	if box.Weight != nil {
		w = *box.Weight
	}
	if box.Volume != nil {
		v = *box.Volume
	}
	body := fmt.Sprintf(`<div class="label"><div class="title">发货箱标 / SHIPMENT LABEL</div>
<div class="row"><span>还货 RMA</span><b>%v</b></div>
<div class="row"><span>箱号 Box No.</span><b>%s</b></div>
<div class="row"><span>出库单</span><b>%s</b></div>
<div class="row"><span>重量 Weight</span><b>%.3f kg</b></div>
<div class="row"><span>材积 Volume</span><b>%.4f m³</b></div>
<div class="sn-title">SN 明细 (%d)</div><ul>`, box.RMANos, box.ShipmentBoxNo, box.OutboundNo, w, v, len(box.SNList))
	for _, sn := range box.SNList {
		body += "<li>" + sn + "</li>"
	}
	body += "</ul></div>"
	return labelPage(box.ShipmentBoxNo, body), nil
}
