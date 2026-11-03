package statemachine_test

import (
	"testing"

	"github.com/boe/rma/internal/enum"
	"github.com/boe/rma/internal/statemachine"
)

// TestSNTransitionMatrix 全量校验 SN 状态机跃迁表（PRD 4.2 / V1.14）。
func TestSNTransitionMatrix(t *testing.T) {
	type tc struct {
		from, to enum.SNStatus
		ctx      statemachine.SNContext
		ok       bool
		name     string
	}
	cases := []tc{
		{enum.SNForecasted, enum.SNReceived, statemachine.SNContext{}, true, "预报→接收"},
		{enum.SNForecasted, enum.SNDiscrepancy, statemachine.SNContext{}, true, "预报→差异"},
		{enum.SNForecasted, enum.SNClosedShort, statemachine.SNContext{}, true, "预报→少收关单"},
		{enum.SNDiscrepancy, enum.SNReceived, statemachine.SNContext{}, true, "差异→接收"},
		{enum.SNReceived, enum.SNInspecting, statemachine.SNContext{}, true, "接收→质检中"},
		{enum.SNInspecting, enum.SNJudged, statemachine.SNContext{}, true, "质检→已判定"},
		{enum.SNJudged, enum.SNOutbound, statemachine.SNContext{}, true, "判定→出库"},
		{enum.SNJudged, enum.SNRetained, statemachine.SNContext{}, true, "判定→留库"},
		{enum.SNOutbound, enum.SNCompleted, statemachine.SNContext{}, true, "出库→完成"},
		{enum.SNOutbound, enum.SNDisposed, statemachine.SNContext{}, true, "出库→已处置"},
		{enum.SNRetained, enum.SNDisposed, statemachine.SNContext{}, true, "留库→已处置"},
		{enum.SNCompleted, enum.SNForecasted, statemachine.SNContext{}, true, "完成→二次返修"},
		// 非法跃迁
		{enum.SNForecasted, enum.SNInspecting, statemachine.SNContext{}, false, "禁止跳级进质检"},
		{enum.SNReceived, enum.SNJudged, statemachine.SNContext{}, false, "禁止跳过质检"},
		{enum.SNJudged, enum.SNCompleted, statemachine.SNContext{}, false, "禁止跳过出库"},
		{enum.SNClosedShort, enum.SNReceived, statemachine.SNContext{}, false, "少收关单为终态"},
		{enum.SNDisposed, enum.SNForecasted, statemachine.SNContext{}, false, "已处置禁止返修"},
		{enum.SNRetained, enum.SNForecasted, statemachine.SNContext{}, false, "留库禁止返修"},
		{enum.SNCompleted, enum.SNRetained, statemachine.SNContext{}, false, "完成不可回留库"},
	}
	for _, c := range cases {
		err := statemachine.CanSNTransit(c.from, c.to, c.ctx)
		if c.ok && err != nil {
			t.Errorf("[%s] 期望允许，实际拒绝: %v", c.name, err)
		}
		if !c.ok && err == nil {
			t.Errorf("[%s] 期望拒绝，实际允许", c.name)
		}
	}
}

// TestRetainedOutboundOnlyForTransfer RETAINED→OUTBOUND 仅调拨可用（R-003）。
func TestRetainedOutboundOnlyForTransfer(t *testing.T) {
	if err := statemachine.CanSNTransit(enum.SNRetained, enum.SNOutbound,
		statemachine.SNContext{OutboundType: enum.OutTransferWarehouse}); err != nil {
		t.Fatalf("调拨应允许: %v", err)
	}
	if err := statemachine.CanSNTransit(enum.SNRetained, enum.SNOutbound,
		statemachine.SNContext{OutboundType: enum.OutScrapDisposal}); err == nil {
		t.Fatal("报废出库不应允许 RETAINED→OUTBOUND（须经 RETAINED→DISPOSED 或处置流程）")
	}
}

// TestBackfillTransition 多收回填须显式允许（RK-03）。
func TestBackfillTransition(t *testing.T) {
	if err := statemachine.CanSNTransit(enum.SNDiscrepancy, enum.SNForecasted,
		statemachine.SNContext{AllowBackfill: true}); err != nil {
		t.Fatalf("回填路径应允许: %v", err)
	}
	if err := statemachine.CanSNTransit(enum.SNDiscrepancy, enum.SNForecasted,
		statemachine.SNContext{}); err == nil {
		t.Fatal("非回填路径应拒绝")
	}
}

// TestRMAStateMachine RMA 6 态逐级推进，禁止跳级/回退。
func TestRMAStateMachine(t *testing.T) {
	chain := []enum.RMAStatus{
		enum.RMAOpen, enum.RMAReceiving, enum.RMAProcessing,
		enum.RMAPendingReturn, enum.RMAReturned, enum.RMAClosed,
	}
	for i := 0; i < len(chain)-1; i++ {
		if err := statemachine.CanRMATransit(chain[i], chain[i+1]); err != nil {
			t.Errorf("合法推进 %s→%s 被拒绝: %v", chain[i], chain[i+1], err)
		}
	}
	// 跳级
	if err := statemachine.CanRMATransit(enum.RMAOpen, enum.RMAProcessing); err == nil {
		t.Error("OPEN→PROCESSING 应被拒绝（跳级）")
	}
	// 回退
	if err := statemachine.CanRMATransit(enum.RMAPendingReturn, enum.RMAProcessing); err == nil {
		t.Error("PENDING_RETURN→PROCESSING 应被拒绝（回退）")
	}
	// 终态
	if err := statemachine.CanRMATransit(enum.RMAClosed, enum.RMAReturned); err == nil {
		t.Error("CLOSED 为终态")
	}
}

// TestEnumDerivations 三段式派生（判定→还货方式 / 原片去向）。
func TestEnumDerivations(t *testing.T) {
	cases := []struct {
		j      enum.Judgment
		method enum.ReturnMethod
		disp   enum.OriginalDisposition
	}{
		{enum.JudgeRepair, enum.ReturnOriginalRepair, enum.OrigShipped},
		{enum.JudgeCID, enum.ReturnCID, enum.OrigShipped},
		{enum.JudgeReplace, enum.ReturnBuffer, enum.OrigRetained},
		{enum.JudgeScrap, enum.ReturnBuffer, enum.OrigScrapped},
		{enum.JudgeTransferRC, enum.ReturnBuffer, enum.OrigTransferredRC},
	}
	for _, c := range cases {
		if got := enum.DeriveReturnMethod(c.j); got != c.method {
			t.Errorf("%s 还货方式期望 %s，实际 %s", c.j, c.method, got)
		}
		if got := enum.DeriveOriginalDisposition(c.j); got != c.disp {
			t.Errorf("%s 原片去向期望 %s，实际 %s", c.j, c.disp, got)
		}
	}
}
