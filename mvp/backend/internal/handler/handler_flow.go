package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/boe/rma/internal/errs"
	"github.com/boe/rma/internal/middleware"
	"github.com/boe/rma/internal/service"
)

// ---------- 差异 ----------

func (h *Handler) ListDiscrepancies(c *gin.Context) {
	p, s := pageParams(c)
	res, err := h.Svc.ListDiscrepancies(c.Query("tab"), c.Query("forecast_no"), c.Query("keyword"), p, s)
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, res)
}

func (h *Handler) GetDiscrepancy(c *gin.Context) {
	rec, err := h.Svc.GetDiscrepancy(c.Param("no"))
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, rec)
}

func (h *Handler) SubmitDiscrepancy(c *gin.Context) {
	var req service.SubmitDiscReq
	if !bind(c, &req) {
		return
	}
	rec, err := h.Svc.SubmitDiscrepancy(c.Param("no"), req, middleware.Operator(c))
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, rec)
}

func (h *Handler) ApproveDiscrepancy(c *gin.Context) {
	var req service.ApproveDiscReq
	if !bind(c, &req) {
		return
	}
	rec, err := h.Svc.ApproveDiscrepancy(c.Param("no"), req, middleware.Operator(c))
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, rec)
}

func (h *Handler) RejectDiscrepancy(c *gin.Context) {
	var req struct {
		Reason string `json:"reason"`
	}
	if !bind(c, &req) {
		return
	}
	if req.Reason == "" {
		middleware.Fail(c, errs.BadRequest("REASON_REQUIRED", "驳回必须填写原因"))
		return
	}
	rec, err := h.Svc.RejectDiscrepancy(c.Param("no"), req.Reason, middleware.Operator(c))
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, rec)
}

// ---------- 质检 ----------

func (h *Handler) ListInspection(c *gin.Context) {
	p, s := pageParams(c)
	res, err := h.Svc.ListInspection(c.Query("status"), c.Query("forecast_no"), p, s)
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, res)
}

func (h *Handler) Judge(c *gin.Context) {
	var req service.JudgeReq
	if !bind(c, &req) {
		return
	}
	it, err := h.Svc.Judge(req, middleware.Operator(c))
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, it)
}

func (h *Handler) JudgeBatch(c *gin.Context) {
	var req struct {
		Items []service.JudgeReq `json:"items"`
	}
	if !bind(c, &req) {
		return
	}
	rows, err := h.Svc.JudgeBatch(req.Items, middleware.Operator(c))
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, gin.H{"count": len(rows), "items": rows})
}

// ---------- 出库 ----------

func (h *Handler) CreateOutbound(c *gin.Context) {
	var req service.CreateOutboundReq
	if !bind(c, &req) {
		return
	}
	ob, err := h.Svc.CreateOutbound(req, middleware.Operator(c))
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	created(c, ob)
}

func (h *Handler) ListOutbounds(c *gin.Context) {
	p, s := pageParams(c)
	res, err := h.Svc.ListOutbounds(c.Query("outbound_type"), c.Query("status"), c.Query("keyword"), p, s)
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, res)
}

func (h *Handler) OutboundDetail(c *gin.Context) {
	d, err := h.Svc.OutboundDetail(c.Param("no"))
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, d)
}

func (h *Handler) ShipOutbound(c *gin.Context) {
	var req service.ShipReq
	if !bind(c, &req) {
		return
	}
	ob, err := h.Svc.ShipOutbound(c.Param("no"), req, middleware.Operator(c))
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, ob)
}

func (h *Handler) ApproveScrap(c *gin.Context) {
	var req struct {
		Opinion string `json:"opinion"`
	}
	if !bind(c, &req) {
		return
	}
	if req.Opinion == "" {
		middleware.Fail(c, errs.BadRequest("OPINION_REQUIRED", "报废审批意见必填"))
		return
	}
	ob, err := h.Svc.ApproveScrap(c.Param("no"), req.Opinion, middleware.Operator(c))
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, ob)
}

func (h *Handler) RejectScrap(c *gin.Context) {
	var req struct {
		Reason string `json:"reason"`
	}
	if !bind(c, &req) {
		return
	}
	ob, err := h.Svc.RejectScrap(c.Param("no"), req.Reason, middleware.Operator(c))
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, ob)
}

func (h *Handler) ResubmitScrap(c *gin.Context) {
	ob, err := h.Svc.ResubmitScrap(c.Param("no"), middleware.Operator(c))
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, ob)
}

func (h *Handler) ConfirmCount(c *gin.Context) {
	ob, err := h.Svc.ConfirmCount(c.Param("no"), middleware.Operator(c))
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, ob)
}

// ---------- 打包 / 发运 ----------

func (h *Handler) CreateShipmentBox(c *gin.Context) {
	var req service.CreateShipBoxReq
	if !bind(c, &req) {
		return
	}
	box, err := h.Svc.CreateShipmentBox(req, middleware.Operator(c))
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	created(c, box)
}

func (h *Handler) AddShipmentItem(c *gin.Context) {
	var req struct {
		SN string `json:"sn"`
	}
	if !bind(c, &req) {
		return
	}
	box, err := h.Svc.AddShipmentItem(c.Param("no"), req.SN, middleware.Operator(c))
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, box)
}

func (h *Handler) SealShipmentBox(c *gin.Context) {
	var req service.SealShipBoxReq
	if !bind(c, &req) {
		return
	}
	box, err := h.Svc.SealShipmentBox(c.Param("no"), req, middleware.Operator(c))
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, box)
}

func (h *Handler) ListShipmentBoxes(c *gin.Context) {
	p, s := pageParams(c)
	res, err := h.Svc.ListShipmentBoxes(c.Query("outbound_no"), p, s)
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, res)
}

func (h *Handler) ShipmentLabel(c *gin.Context) {
	html, err := h.Svc.ShipmentLabelHTML(c.Param("no"))
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}
