package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/boe/rma/internal/middleware"
	"github.com/boe/rma/internal/service"
)

// ---------- 还货 ----------

func (h *Handler) CreateReturn(c *gin.Context) {
	var req service.CreateReturnReq
	if !bind(c, &req) {
		return
	}
	plan, err := h.Svc.CreateReturn(req, middleware.Operator(c))
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	created(c, plan)
}

func (h *Handler) ReturnProgress(c *gin.Context) {
	v, err := h.Svc.GetReturnProgress(c.Param("no"))
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, v)
}

func (h *Handler) ReturnBoard(c *gin.Context) {
	p, s := pageParams(c)
	res, err := h.Svc.ListReturnBoard(c.Query("status"), c.Query("keyword"), p, s)
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, res)
}

// ---------- RETAINED 处置 ----------

func (h *Handler) DisposeRetained(c *gin.Context) {
	var req service.DisposeReq
	if !bind(c, &req) {
		return
	}
	res, err := h.Svc.DisposeRetained(c.Param("sn"), req, middleware.Operator(c))
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, res)
}

// ---------- BUFFER ----------

func (h *Handler) BufferInbound(c *gin.Context) {
	var req service.InboundReq
	if !bind(c, &req) {
		return
	}
	b, err := h.Svc.BufferInbound(req, middleware.Operator(c))
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	created(c, b)
}

func (h *Handler) BufferStock(c *gin.Context) {
	rows, err := h.Svc.BufferStockSummary(c.Query("pn_code"))
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, gin.H{"items": rows})
}

func (h *Handler) BufferSNList(c *gin.Context) {
	p, s := pageParams(c)
	res, err := h.Svc.ListBufferSN(c.Query("pn_code"), c.Query("status"), p, s)
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, res)
}

func (h *Handler) ScrapBuffer(c *gin.Context) {
	var req struct {
		Reason string `json:"reason"`
	}
	if !bind(c, &req) {
		return
	}
	if err := h.Svc.ScrapBuffer(c.Param("sn"), req.Reason, middleware.Operator(c)); err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, gin.H{"result": "备件已报废"})
}

func (h *Handler) RequestSubstitute(c *gin.Context) {
	var req service.SubstituteReq
	if !bind(c, &req) {
		return
	}
	ap, err := h.Svc.RequestSubstitute(req, middleware.Operator(c))
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	created(c, ap)
}

func (h *Handler) ApproveSubstitute(c *gin.Context) {
	var req struct {
		Note    string `json:"note"`
		Approve bool   `json:"approve"`
	}
	if !bind(c, &req) {
		return
	}
	ap, err := h.Svc.ApproveSubstitute(c.Param("no"), req.Note, middleware.Operator(c), req.Approve)
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, ap)
}

// ---------- 报表 ----------

func (h *Handler) ReportRMA(c *gin.Context) {
	p, s := pageParams(c)
	res, err := h.Svc.ReportRMA(c.Query("customer"), p, s)
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, res)
}

func (h *Handler) ReportSKU(c *gin.Context) {
	rows, err := h.Svc.ReportSKU()
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, gin.H{"items": rows})
}

func (h *Handler) ReportTrend(c *gin.Context) {
	rows, err := h.Svc.ReportTrend(c.Query("period"), 12)
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, gin.H{"items": rows})
}

func (h *Handler) ReportDiscrepancy(c *gin.Context) {
	res, err := h.Svc.ReportDiscrepancy()
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, res)
}

func (h *Handler) ReportDetail(c *gin.Context) {
	p, s := pageParams(c)
	res, err := h.Svc.ReportDetail(c.Query("status"), c.Query("rma_no"), p, s)
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, res)
}

func (h *Handler) ReportTAT(c *gin.Context) {
	res, err := h.Svc.ReportTAT(c.Query("customer"))
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, res)
}

func (h *Handler) TATAlerts(c *gin.Context) {
	rows, err := h.Svc.ListTATAlerts()
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, gin.H{"items": rows})
}

func (h *Handler) RefreshTAT(c *gin.Context) {
	n, err := h.Svc.RefreshTAT()
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, gin.H{"refreshed": n})
}

// ---------- SN 轨迹 / 审计 ----------

func (h *Handler) SNTrace(c *gin.Context) {
	res, err := h.Svc.SNTrace(c.Param("sn"))
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, res)
}

func (h *Handler) ListAuditLogs(c *gin.Context) {
	p, s := pageParams(c)
	res, err := h.Svc.ListAuditLogs(c.Query("action"), c.Query("biz_no"), p, s)
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, res)
}

// Health 健康检查。
func Health(c *gin.Context) {
	ok(c, gin.H{"status": "ok"})
}
