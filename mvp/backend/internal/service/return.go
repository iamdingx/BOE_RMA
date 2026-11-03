package service

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/boe/rma/internal/enum"
	"github.com/boe/rma/internal/errs"
	"github.com/boe/rma/internal/model"
	"github.com/boe/rma/internal/seq"
)

// ---------- E11 BUFFER 缓冲备件 ----------

// InboundReq 备件入库（BS-01/07）。
type InboundReq struct {
	SN     string `json:"sn"`
	PNCode string `json:"pn_code"`
	Source string `json:"source"` // NEW_PURCHASE / REPAIRED_GOOD
}

// BufferInbound 缓冲备件入库（AVAILABLE）。PN 必须存在于主数据。
func (s *Service) BufferInbound(req InboundReq, operator string) (*model.BufferStock, error) {
	sn := normSN(req.SN)
	pn := trimAll(req.PNCode)
	if sn == "" || pn == "" {
		return nil, errs.BadRequest("INVALID_PARAM", "SN 与料号必填")
	}
	if req.Source != "NEW_PURCHASE" && req.Source != "REPAIRED_GOOD" {
		return nil, errs.ErrInvalidEnum("source", req.Source)
	}
	var out *model.BufferStock
	err := s.Tx(func(tx *gorm.DB) error {
		var cnt int64
		if err := tx.Model(&model.PartNumber{}).
			Where("tenant_id = ? AND pn_code = ?", s.Tenant, pn).Count(&cnt).Error; err != nil {
			return err
		}
		if cnt == 0 {
			return errs.BadRequest("PN_NOT_FOUND", "料号 "+pn+" 不在主数据中，禁止入库")
		}
		var dup int64
		if err := tx.Model(&model.BufferStock{}).
			Where("tenant_id = ? AND sn = ?", s.Tenant, sn).Count(&dup).Error; err != nil {
			return err
		}
		if dup > 0 {
			return errs.Conflict("BUFFER_SN_EXISTS", "备件 "+sn+" 已存在库存记录")
		}
		out = &model.BufferStock{
			TenantID: s.Tenant, SN: sn, PNCode: pn, Source: req.Source,
			StockStatus: string(enum.BufferAvailable), InboundAt: s.now(), InboundBy: operator,
		}
		return tx.Create(out).Error
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// BufferStockRow 库存汇总行。
type BufferStockRow struct {
	PNCode    string `json:"pn_code"`
	Available int64  `json:"available"`
	Reserved  int64  `json:"reserved"`
	Out       int64  `json:"out"`
	Scrapped  int64  `json:"scrapped"`
}

// BufferStockSummary 按 PN 汇总库存（BS-02/05）。
func (s *Service) BufferStockSummary(pn string) ([]BufferStockRow, error) {
	type row struct {
		PNCode      string
		StockStatus string
		Cnt         int64
	}
	q := s.DB.Model(&model.BufferStock{}).Select("pn_code, stock_status, count(*) as cnt").Where("tenant_id = ?", s.Tenant)
	if pn != "" {
		q = q.Where("pn_code = ?", pn)
	}
	var rows []row
	if err := q.Group("pn_code, stock_status").Scan(&rows).Error; err != nil {
		return nil, err
	}
	agg := map[string]*BufferStockRow{}
	for _, r := range rows {
		v, ok := agg[r.PNCode]
		if !ok {
			v = &BufferStockRow{PNCode: r.PNCode}
			agg[r.PNCode] = v
		}
		switch enum.BufferStatus(r.StockStatus) {
		case enum.BufferAvailable:
			v.Available = r.Cnt
		case enum.BufferReserved:
			v.Reserved = r.Cnt
		case enum.BufferOut:
			v.Out = r.Cnt
		case enum.BufferScrapped:
			v.Scrapped = r.Cnt
		}
	}
	out := make([]BufferStockRow, 0, len(agg))
	for _, v := range agg {
		out = append(out, *v)
	}
	return out, nil
}

// ListBufferSN 明细 SN 查询（BS-05）。
func (s *Service) ListBufferSN(pn, status string, page, size int) (PageResult, error) {
	q := s.DB.Model(&model.BufferStock{}).Where("tenant_id = ?", s.Tenant)
	if pn != "" {
		q = q.Where("pn_code = ?", pn)
	}
	if status != "" {
		q = q.Where("stock_status = ?", status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return PageResult{}, err
	}
	var rows []model.BufferStock
	q2, p, sz := paged(q, page, size)
	if err := q2.Order("inbound_at DESC").Find(&rows).Error; err != nil {
		return PageResult{}, err
	}
	return wrapPage(rows, total, p, sz), nil
}

// reserveBuffer 事务内原子预占（RK-10 防并发超扣）。
// 采用条件更新（WHERE stock_status='AVAILABLE'）保证同一 SN 只可能被一个请求预占。
func (s *Service) reserveBuffer(tx *gorm.DB, sn, holdNo string) error {
	res := tx.Model(&model.BufferStock{}).
		Where("tenant_id = ? AND sn = ? AND stock_status = ?", s.Tenant, sn, string(enum.BufferAvailable)).
		Updates(map[string]any{
			"stock_status": string(enum.BufferReserved),
			"reserved_by":  holdNo,
			"reserved_at":  s.now(),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errs.ErrBufferConcurrent(sn)
	}
	return nil
}

// pickSamePN 取同 PN 首个可用备件（BS-08 默认同 PN）。
func (s *Service) pickSamePN(tx *gorm.DB, pn string) (*model.BufferStock, error) {
	var b model.BufferStock
	err := tx.Where("tenant_id = ? AND pn_code = ? AND stock_status = ?", s.Tenant, pn, string(enum.BufferAvailable)).
		Order("inbound_at ASC").First(&b).Error
	if err != nil {
		return nil, errs.ErrBufferInsufficient(pn)
	}
	return &b, nil
}

// ScrapBuffer 备件报废（BS-09）：单独流程，从 AVAILABLE 移除，不进 SN 状态机。
func (s *Service) ScrapBuffer(sn, reason, operator string) error {
	return s.Tx(func(tx *gorm.DB) error {
		var b model.BufferStock
		if err := tx.Where("tenant_id = ? AND sn = ?", s.Tenant, normSN(sn)).First(&b).Error; err != nil {
			return errs.NotFound("备件 " + sn)
		}
		if b.StockStatus != string(enum.BufferAvailable) {
			return errs.ErrStateNotAllowed("备件报废", b.StockStatus)
		}
		if err := tx.Model(&model.BufferStock{}).
			Where("tenant_id = ? AND sn = ?", s.Tenant, b.SN).
			Update("stock_status", string(enum.BufferScrapped)).Error; err != nil {
			return err
		}
		return s.audit(tx, enum.AuditRetainedDispose, "buffer_stock", b.SN, "stock_status",
			string(enum.BufferAvailable), string(enum.BufferScrapped), reason, operator, "")
	})
}

// ---------- 跨 PN 替代审批（BS-08 / M-03 / RK-12） ----------

// SubstituteReq 跨 PN 替代申请入参（申请本身由业务动作发起）。
type SubstituteReq struct {
	OriginalSN   string `json:"original_sn"`
	SubstituteSN string `json:"substitute_sn"`
	Note         string `json:"note"`
}

// RequestSubstitute 发起跨 PN 替代审批（PENDING）。
func (s *Service) RequestSubstitute(req SubstituteReq, operator string) (*model.BufferSubstituteApproval, error) {
	var out *model.BufferSubstituteApproval
	err := s.Tx(func(tx *gorm.DB) error {
		it, err := s.getSN(tx, req.OriginalSN)
		if err != nil {
			return err
		}
		var b model.BufferStock
		if err := tx.Where("tenant_id = ? AND sn = ?", s.Tenant, normSN(req.SubstituteSN)).First(&b).Error; err != nil {
			return errs.NotFound("备件 " + req.SubstituteSN)
		}
		if b.PNCode == it.PNCode {
			return errs.BadRequest("SAME_PN", "同 PN 备件无需跨 PN 替代审批")
		}
		no, err := seq.NextBizNo(tx, s.Tenant, "BSA", "BSA-", 4, s.now())
		if err != nil {
			return err
		}
		out = &model.BufferSubstituteApproval{
			TenantID: s.Tenant, ApprovalNo: no, OriginalSN: it.SN, OriginalPN: it.PNCode,
			SubstituteSN: b.SN, SubstitutePN: b.PNCode, Status: string(enum.ApprovalPending),
			Note: req.Note, RequestedBy: operator, CreatedAt: s.now(),
		}
		return tx.Create(out).Error
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ApproveSubstitute 跨 PN 替代审批（仅项目人员，无需客户确认）。
func (s *Service) ApproveSubstitute(approvalNo, note, approver string, approve bool) (*model.BufferSubstituteApproval, error) {
	var out model.BufferSubstituteApproval
	err := s.Tx(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ? AND approval_no = ?", s.Tenant, approvalNo).First(&out).Error; err != nil {
			return errs.NotFound("审批单 " + approvalNo)
		}
		if out.Status != string(enum.ApprovalPending) {
			return errs.ErrStateNotAllowed("审批跨PN替代", out.Status)
		}
		st := enum.ApprovalApproved
		if !approve {
			st = enum.ApprovalRejected
		}
		if err := tx.Model(&model.BufferSubstituteApproval{}).
			Where("tenant_id = ? AND approval_no = ?", s.Tenant, approvalNo).
			Updates(map[string]any{
				"status": string(st), "approver": approver,
				"approved_at": s.now(), "note": note,
			}).Error; err != nil {
			return err
		}
		out.Status = string(st)
		return s.audit(tx, enum.AuditBufferSubstitute, "buffer_substitute_approval", approvalNo, "status",
			string(enum.ApprovalPending), string(st), note, approver, approver)
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ---------- 还货编排（RT-01/02/03 / OB-11） ----------

// CreateReturnReq 还货发起入参。
type CreateReturnReq struct {
	RMANo      string `json:"rma_no"`
	OriginalSN string `json:"original_sn"`
	BufferSN   string `json:"buffer_sn"`
}

// ReturnPlan 还货编排结果。
type ReturnPlan struct {
	Return      *model.ReturnRecord `json:"return_record"`
	ReturnOut   *model.OutboundOrder `json:"return_outbound"`
	DisposalOut *model.OutboundOrder `json:"disposal_outbound,omitempty"`
}

// CreateReturn 按判定自动带出还货方式并编排出库（三段式）。
func (s *Service) CreateReturn(req CreateReturnReq, operator string) (*ReturnPlan, error) {
	var plan ReturnPlan
	err := s.Tx(func(tx *gorm.DB) error {
		it, err := s.getSN(tx, req.OriginalSN)
		if err != nil {
			return err
		}
		if enum.SNStatus(it.Status) != enum.SNJudged {
			return errs.ErrStateNotAllowed("发起还货", it.Status)
		}
		if it.JudgmentResult == "" {
			return errs.BadRequest("NO_JUDGMENT", "该 SN 尚未判定，无法确定还货方式")
		}
		judgment := enum.Judgment(it.JudgmentResult)
		method := enum.DeriveReturnMethod(judgment)
		disp := enum.DeriveOriginalDisposition(judgment)
		rmaNo := it.RMANo
		if req.RMANo != "" {
			rmaNo = req.RMANo
		}
		rno, err := seq.NextBizNo(tx, s.Tenant, "RETURN", "RT-", 4, s.now())
		if err != nil {
			return err
		}
		rec := &model.ReturnRecord{
			TenantID: s.Tenant, ReturnNo: rno, RMANo: rmaNo, ForecastNo: it.ForecastNo,
			OriginalSN: it.SN, ReturnMethod: string(method),
			OriginalDisposition: string(disp), Status: "PENDING",
			Operator: operator, CreatedAt: s.now(),
		}

		switch method {
		case enum.ReturnOriginalRepair, enum.ReturnCID:
			rec.ReturnedSN = it.SN
			if err := tx.Create(rec).Error; err != nil {
				return err
			}
			ob, err := s.createOutboundTx(tx, CreateOutboundReq{
				Type: enum.OutShipToCustomer, SNs: []string{it.SN}, RMANo: rmaNo,
				CIDFlag: method == enum.ReturnCID,
			}, operator)
			if err != nil {
				return err
			}
			plan.ReturnOut = ob
			if err := tx.Model(&model.ReturnRecord{}).
				Where("tenant_id = ? AND return_no = ?", s.Tenant, rno).
				Update("outbound_no", ob.OutboundNo).Error; err != nil {
				return err
			}
			rec.OutboundNo = ob.OutboundNo

		case enum.ReturnBuffer:
			// 选备件：指定 or 自动同 PN
			var picked model.BufferStock
			if req.BufferSN != "" {
				if err := tx.Where("tenant_id = ? AND sn = ?", s.Tenant, normSN(req.BufferSN)).First(&picked).Error; err != nil {
					return errs.NotFound("备件 " + req.BufferSN)
				}
				if picked.StockStatus != string(enum.BufferAvailable) {
					return errs.ErrStateNotAllowed("BUFFER 还货选件", picked.StockStatus)
				}
				// 跨 PN 必须已有 APPROVED 审批（BS-08）
				if picked.PNCode != it.PNCode {
					var ap model.BufferSubstituteApproval
					e := tx.Where("tenant_id = ? AND original_sn = ? AND substitute_sn = ? AND status = ?",
						s.Tenant, it.SN, picked.SN, string(enum.ApprovalApproved)).First(&ap).Error
					if e != nil {
						return errs.Conflict("SUBSTITUTE_APPROVAL_REQUIRED",
							"跨 PN 替代还货需项目人员审批通过后执行（可先调用 /buffer/substitute 发起审批）")
					}
				}
			} else {
				b, err := s.pickSamePN(tx, it.PNCode)
				if err != nil {
					return err
				}
				picked = *b
			}
			// 原子预占（防并发超扣）
			if err := s.reserveBuffer(tx, picked.SN, rno); err != nil {
				return err
			}
			rec.ReturnedSN = picked.SN
			if err := tx.Create(rec).Error; err != nil {
				return err
			}
			ob, err := s.createOutboundTx(tx, CreateOutboundReq{
				Type: enum.OutShipToCustomer, SNs: []string{picked.SN}, RMANo: rmaNo,
			}, operator)
			if err != nil {
				return err
			}
			plan.ReturnOut = ob
			if err := tx.Model(&model.ReturnRecord{}).
				Where("tenant_id = ? AND return_no = ?", s.Tenant, rno).
				Update("outbound_no", ob.OutboundNo).Error; err != nil {
				return err
			}
			rec.OutboundNo = ob.OutboundNo

			// 原片处置出库（OB-11）
			switch judgment {
			case enum.JudgeScrap:
				dob, err := s.createOutboundTx(tx, CreateOutboundReq{
					Type: enum.OutScrapDisposal, SNs: []string{it.SN}, RMANo: rmaNo,
				}, operator)
				if err != nil {
					return err
				}
				plan.DisposalOut = dob
			case enum.JudgeTransferRC:
				dob, err := s.createOutboundTx(tx, CreateOutboundReq{
					Type: enum.OutShipToRC, SNs: []string{it.SN}, RMANo: rmaNo,
				}, operator)
				if err != nil {
					return err
				}
				plan.DisposalOut = dob
			}
		}
		plan.Return = rec
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &plan, nil
}

// ---------- RETAINED 处置（RT-09 / BS-06） ----------

// DisposeReq RETAINED 处置入参。
type DisposeReq struct {
	Disposition enum.RetainedDisposition `json:"retained_disposition"`
	Note        string                   `json:"note"`
}

// DisposeRetained 原片后续处置（4 条路径）。
// SCRAPPED → 报废出库单（需审批）；RETURN_VENDOR → 退供应商出库（SHIP_TO_RC 承载，见 R-022）；
// REPAIR_TO_BUFFER → 直接入 buffer（同 SN 双身份，RK-09）；LONG_RETAIN → 保持 RETAINED（不强制收尾）。
func (s *Service) DisposeRetained(sn string, req DisposeReq, operator string) (map[string]any, error) {
	if !req.Disposition.Valid() {
		return nil, errs.ErrInvalidEnum("retained_disposition", string(req.Disposition))
	}
	out := map[string]any{}
	err := s.Tx(func(tx *gorm.DB) error {
		it, err := s.getSN(tx, sn)
		if err != nil {
			return err
		}
		if enum.SNStatus(it.Status) != enum.SNRetained {
			return errs.ErrStateNotAllowed("原片处置", it.Status)
		}
		if err := tx.Model(&model.ReturnRecord{}).
			Where("tenant_id = ? AND original_sn = ? AND original_disposition = ?",
				s.Tenant, it.SN, string(enum.OrigRetained)).
			Update("retained_disposition", string(req.Disposition)).Error; err != nil {
			return err
		}
		switch req.Disposition {
		case enum.RetLongRetain:
			out["result"] = "LONG_RETAIN 保持留库，未收尾"
		case enum.RetRepairToBuffer:
			// BS-06：修好入 buffer，SN 直接 DISPOSED（同 SN 双身份）
			var cnt int64
			if err := tx.Model(&model.BufferStock{}).
				Where("tenant_id = ? AND sn = ?", s.Tenant, it.SN).Count(&cnt).Error; err != nil {
				return err
			}
			if cnt == 0 {
				if err := tx.Create(&model.BufferStock{
					TenantID: s.Tenant, SN: it.SN, PNCode: it.PNCode, Source: "REPAIRED_GOOD",
					StockStatus: string(enum.BufferAvailable), InboundAt: s.now(), InboundBy: operator,
				}).Error; err != nil {
					return err
				}
			}
			if _, err := s.transit(tx, it.SN, enum.SNDisposed, statemachineCtxFor(""), operator); err != nil {
				return err
			}
			if err := s.setSNFields(tx, it.SN, map[string]any{"return_method": nil}); err != nil {
				return err
			}
			out["result"] = "已入 BUFFER 库存（AVAILABLE），原片 SN 置 DISPOSED"
		case enum.RetScrapped, enum.RetReturnVendor:
			obType := enum.OutScrapDisposal
			if req.Disposition == enum.RetReturnVendor {
				obType = enum.OutShipToRC // 退供应商以 SHIP_TO_RC 承载（R-022）
			}
			ob, err := s.createOutboundTx(tx, CreateOutboundReq{
				Type: obType, SNs: []string{it.SN}, RMANo: it.RMANo,
			}, operator)
			if err != nil {
				return err
			}
			out["outbound"] = ob
			out["result"] = "已生成处置出库单，审批/发运后原片置 DISPOSED"
		}
		return s.audit(tx, enum.AuditRetainedDispose, "sn_item", it.SN, "retained_disposition",
			"", string(req.Disposition), req.Note, operator, "")
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

var _ = fmt.Sprint
