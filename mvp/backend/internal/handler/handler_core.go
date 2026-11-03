// Package handler HTTP 层：只做参数绑定与响应，不含业务规则。
package handler

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/boe/rma/internal/auth"
	"github.com/boe/rma/internal/enum"
	"github.com/boe/rma/internal/errs"
	"github.com/boe/rma/internal/middleware"
	"github.com/boe/rma/internal/service"
	"github.com/boe/rma/internal/storage"
)

// Handler 聚合依赖。
type Handler struct {
	Svc    *service.Service
	Signer *auth.Signer
	Store  storage.Storage
}

func New(svc *service.Service, signer *auth.Signer, st storage.Storage) *Handler {
	return &Handler{Svc: svc, Signer: signer, Store: st}
}

// ---------- 通用 ----------

func pageParams(c *gin.Context) (int, int) {
	p, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	s, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	return p, s
}

func bind(c *gin.Context, out any) bool {
	if err := c.ShouldBindJSON(out); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "INVALID_BODY", "message": "请求体格式错误：" + err.Error()})
		return false
	}
	return true
}

func ok(c *gin.Context, v any) { c.JSON(http.StatusOK, v) }
func created(c *gin.Context, v any) { c.JSON(http.StatusCreated, v) }

// ---------- 认证 ----------

func (h *Handler) Login(c *gin.Context) {
	var req service.LoginReq
	if !bind(c, &req) {
		return
	}
	res, err := h.Svc.Login(req, h.Signer)
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, res)
}

func (h *Handler) Me(c *gin.Context) {
	cl, _ := middleware.Claims(c)
	ok(c, gin.H{"username": cl.Username, "role": cl.Role, "tenant_id": cl.TenantID})
}

// ---------- 主数据 ----------

func (h *Handler) ListModels(c *gin.Context) {
	rows, err := h.Svc.ListModels()
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, gin.H{"items": rows})
}

func (h *Handler) CreateModel(c *gin.Context) {
	var req struct {
		Code string `json:"model_code"`
		Name string `json:"model_name"`
	}
	if !bind(c, &req) {
		return
	}
	m, err := h.Svc.CreateModel(req.Code, req.Name)
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	created(c, m)
}

func (h *Handler) ListPNs(c *gin.Context) {
	rows, err := h.Svc.ListPNs(c.Query("model_code"))
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, gin.H{"items": rows})
}

func (h *Handler) CreatePN(c *gin.Context) {
	var req struct {
		Code      string `json:"pn_code"`
		ModelCode string `json:"model_code"`
		Desc      string `json:"description"`
	}
	if !bind(c, &req) {
		return
	}
	p, err := h.Svc.CreatePN(req.Code, req.ModelCode, req.Desc)
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	created(c, p)
}

func (h *Handler) ImportMaster(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		middleware.Fail(c, errs.BadRequest("FILE_REQUIRED", "请上传 Excel 文件"))
		return
	}
	f, err := fh.Open()
	if err != nil {
		middleware.Fail(c, errs.BadRequest("FILE_ERROR", "文件读取失败"))
		return
	}
	defer f.Close()
	res, err := h.Svc.ImportMaster(f)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "MASTER_IMPORT_HAS_ERRORS", "message": err.Error(), "detail": res})
		return
	}
	ok(c, res)
}

func (h *Handler) ListUsers(c *gin.Context) {
	rows, err := h.Svc.ListUsers()
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, gin.H{"items": rows})
}

func (h *Handler) CreateUser(c *gin.Context) {
	var req struct {
		Username string     `json:"username"`
		Password string     `json:"password"`
		Display  string     `json:"display_name"`
		Role     enum.Role  `json:"role"`
	}
	if !bind(c, &req) {
		return
	}
	u, err := h.Svc.CreateUser(req.Username, req.Password, req.Display, req.Role)
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	created(c, u)
}

// ---------- 照片 ----------

func (h *Handler) UploadPhoto(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		middleware.Fail(c, errs.BadRequest("FILE_REQUIRED", "请上传照片文件"))
		return
	}
	if fh.Size > service.MaxPhotoSize {
		middleware.Fail(c, errs.PayloadTooLarge("单张照片不得超过 10MB"))
		return
	}
	bizType := c.PostForm("biz_type")
	bizRef := c.PostForm("biz_ref_no")
	if bizRef == "" {
		middleware.Fail(c, errs.BadRequest("INVALID_PARAM", "biz_ref_no 必填"))
		return
	}
	f, err := fh.Open()
	if err != nil {
		middleware.Fail(c, errs.BadRequest("FILE_ERROR", "文件读取失败"))
		return
	}
	defer f.Close()
	// 读入内存以便同时落对象存储（≤10MB 可控）
	buf := make([]byte, 0, fh.Size)
	b, err := io.ReadAll(io.LimitReader(f, service.MaxPhotoSize+1))
	if err != nil {
		middleware.Fail(c, errs.BadRequest("FILE_ERROR", "文件读取失败"))
		return
	}
	buf = append(buf, b...)
	if int64(len(buf)) > service.MaxPhotoSize {
		middleware.Fail(c, errs.PayloadTooLarge("单张照片不得超过 10MB"))
		return
	}
	key := fmt.Sprintf("%s/%s/%s/%d-%s", h.Svc.Tenant, bizType, bizRef, time.Now().UnixNano(), fh.Filename)
	url, err := h.Store.Put(c.Request.Context(), key, bytes.NewReader(buf), int64(len(buf)), fh.Header.Get("Content-Type"))
	if err != nil {
		middleware.Fail(c, errs.Internal("对象存储写入失败"))
		return
	}
	p, err := h.Svc.SavePhoto(service.SavePhotoReq{
		BizType: enum.PhotoBizType(bizType), BizRefNo: bizRef,
		SN: c.PostForm("sn"), PNCode: c.PostForm("pn_code"),
		FileName: fh.Filename, ContentType: fh.Header.Get("Content-Type"),
		Size: int64(len(buf)), ObjectKey: key, OriginalURL: url,
		UploadedBy: middleware.Operator(c),
	})
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	created(c, p)
}

func (h *Handler) ListPhotos(c *gin.Context) {
	rows, err := h.Svc.ListPhotos(c.Query("biz_type"), c.Query("biz_ref_no"))
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, gin.H{"items": rows})
}

// ---------- 预报 ----------

func (h *Handler) CreateForecast(c *gin.Context) {
	var req service.CreateForecastReq
	if !bind(c, &req) {
		return
	}
	fo, err := h.Svc.CreateForecast(req, middleware.Operator(c))
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	created(c, fo)
}

func (h *Handler) ListForecast(c *gin.Context) {
	p, s := pageParams(c)
	res, err := h.Svc.ListForecast(c.Query("status"), c.Query("customer"), c.Query("keyword"), p, s)
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, res)
}

func (h *Handler) GetForecast(c *gin.Context) {
	res, err := h.Svc.GetForecast(c.Param("no"))
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, res)
}

func (h *Handler) ImportForecastSN(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		middleware.Fail(c, errs.BadRequest("FILE_REQUIRED", "请上传 Excel 文件"))
		return
	}
	f, err := fh.Open()
	if err != nil {
		middleware.Fail(c, errs.BadRequest("FILE_ERROR", "文件读取失败"))
		return
	}
	defer f.Close()
	items, err := h.Svc.ImportForecastSN(f)
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, gin.H{"count": len(items), "items": items})
}

func (h *Handler) ForecastSNTemplate(c *gin.Context) {
	b, err := h.Svc.ForecastSNTemplate()
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="forecast_sn_template.xlsx"`)
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", b)
}

func (h *Handler) ExportForecast(c *gin.Context) {
	b, err := h.Svc.ExportForecast(c.Param("no"))
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="forecast_%s.xlsx"`, c.Param("no")))
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", b)
}

// ---------- 物流允收 ----------

func (h *Handler) CreateBatch(c *gin.Context) {
	var req service.CreateBatchReq
	if !bind(c, &req) {
		return
	}
	b, err := h.Svc.CreateBatch(req, middleware.Operator(c))
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	created(c, b)
}

func (h *Handler) ListBatches(c *gin.Context) {
	p, s := pageParams(c)
	res, err := h.Svc.ListBatches(c.Query("forecast_no"), p, s)
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, res)
}

// ---------- 收货箱 / kitting ----------

func (h *Handler) CreateBox(c *gin.Context) {
	var req struct {
		BatchNo string `json:"batch_no"`
	}
	if !bind(c, &req) {
		return
	}
	b, err := h.Svc.CreateBox(req.BatchNo, middleware.Operator(c))
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	created(c, b)
}

func (h *Handler) Scan(c *gin.Context) {
	var req service.ScanReq
	if !bind(c, &req) {
		return
	}
	res, err := h.Svc.Scan(req, middleware.Operator(c))
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, res)
}

func (h *Handler) SealBox(c *gin.Context) {
	b, err := h.Svc.SealBox(c.Param("no"), middleware.Operator(c))
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, b)
}

func (h *Handler) BoxDetail(c *gin.Context) {
	d, err := h.Svc.BoxDetail(c.Param("no"))
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, d)
}

func (h *Handler) ListBoxes(c *gin.Context) {
	p, s := pageParams(c)
	res, err := h.Svc.ListBoxes(c.Query("batch_no"), p, s)
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	ok(c, res)
}

func (h *Handler) ReceiveLabel(c *gin.Context) {
	html, err := h.Svc.ReceiveLabelHTML(c.Param("no"))
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}
