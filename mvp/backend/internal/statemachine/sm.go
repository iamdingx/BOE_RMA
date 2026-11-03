// Package statemachine 两套状态机的唯一权威实现。
// 所有状态变更必须经此包校验，禁止业务代码直接写 status 字段。
package statemachine

import (
	"github.com/boe/rma/internal/enum"
	"github.com/boe/rma/internal/errs"
)

// SNContext 状态跃迁上下文（用于受限跃迁判定）。
type SNContext struct {
	// OutboundType 当目标状态为 OUTBOUND 时必填（如 RETAINED→OUTBOUND 仅允许调拨）。
	OutboundType enum.OutboundType
	// AllowBackfill 多收/未预报实物在差异审批通过后回填预报，需先置 FORECASTED 再 RECEIVED（RK-03）。
	AllowBackfill bool
}

// snTransitions SN 状态机跃迁表（PRD 4.2，V1.14）。
var snTransitions = map[enum.SNStatus][]enum.SNStatus{
	enum.SNForecasted:  {enum.SNReceived, enum.SNDiscrepancy, enum.SNClosedShort},
	enum.SNDiscrepancy: {enum.SNReceived},
	enum.SNReceived:    {enum.SNInspecting, enum.SNOutbound},
	enum.SNInspecting:  {enum.SNJudged},
	enum.SNJudged:      {enum.SNOutbound, enum.SNRetained},
	enum.SNOutbound:    {enum.SNCompleted, enum.SNDisposed},
	enum.SNRetained:    {enum.SNDisposed, enum.SNOutbound}, // OUTBOUND 受限：仅调拨（R-003）
	enum.SNCompleted:   {enum.SNForecasted},                // 二次返修（仅 COMPLETED，SN-02）
	enum.SNClosedShort: {},                                 // 终态
	enum.SNDisposed:    {},                                 // 终态
}

// CanSNTransit 校验 SN 状态跃迁合法性。
func CanSNTransit(from, to enum.SNStatus, ctx SNContext) error {
	if !from.Valid() || !to.Valid() {
		return errs.ErrInvalidEnum("sn_status", string(from)+"→"+string(to))
	}
	// 回填专用跃迁：DISCREPANCY → FORECASTED（仅差异审批回填路径，RK-03）
	if from == enum.SNDiscrepancy && to == enum.SNForecasted {
		if ctx.AllowBackfill {
			return nil
		}
		return errs.Conflict("BACKFILL_REQUIRED", "差异未闭环，不允许直接回到已预报")
	}
	allowed := snTransitions[from]
	for _, a := range allowed {
		if a == to {
			// 受限跃迁：RETAINED→OUTBOUND 与 RECEIVED/JUDGED→OUTBOUND 均需判定出库类型
			if to == enum.SNOutbound {
				if ctx.OutboundType == enum.OutTransferWarehouse && from == enum.SNRetained {
					return nil
				}
				if ctx.OutboundType == "" || ctx.OutboundType == enum.OutTransferWarehouse {
					return nil // 调拨允许 RECEIVED/JUDGED/RETAINED
				}
				// 非调拨出库：RETAINED 不允许
				if from == enum.SNRetained {
					return errs.Conflict("RETAINED_TRANSFER_ONLY", "留库待定原片仅可通过调拨出库")
				}
			}
			return nil
		}
	}
	return errs.ErrIllegalTransition(string(from), string(to))
}

// MustTransit 便捷判定（布尔）。
func CanSNTransitBool(from, to enum.SNStatus, ctx SNContext) bool {
	return CanSNTransit(from, to, ctx) == nil
}

// rmaTransitions RMA 状态机（PRD 4.2-B）。
var rmaTransitions = map[enum.RMAStatus][]enum.RMAStatus{
	enum.RMAOpen:          {enum.RMAReceiving},
	enum.RMAReceiving:     {enum.RMAProcessing},
	enum.RMAProcessing:    {enum.RMAPendingReturn},
	enum.RMAPendingReturn: {enum.RMAReturned},
	enum.RMAReturned:      {enum.RMAClosed},
	enum.RMAClosed:        {},
}

// CanRMATransit 校验 RMA 状态跃迁（仅允许逐级推进，禁止跳级/回退）。
func CanRMATransit(from, to enum.RMAStatus) error {
	for _, a := range rmaTransitions[from] {
		if a == to {
			return nil
		}
	}
	return errs.ErrIllegalTransition(string(from), string(to))
}

// RMARank 返回状态序号，用于聚合推进时比较当前进度。
func RMARank(s enum.RMAStatus) int {
	for i, v := range enum.AllRMAStatus {
		if v == s {
			return i
		}
	}
	return -1
}
