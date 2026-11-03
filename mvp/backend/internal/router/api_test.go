package router_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/boe/rma/internal/auth"
	"github.com/boe/rma/internal/handler"
	"github.com/boe/rma/internal/router"
	"github.com/boe/rma/internal/service"
	"github.com/boe/rma/internal/storage"
	"github.com/boe/rma/internal/store"
)

type apiEnv struct {
	r      *gin.Engine
	tokens map[string]string
}

func newAPI(t *testing.T) *apiEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:apidb%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := store.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := store.Seed(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)

	st := storage.NewMem()
	svc := service.New(db, "BOE_US", st)
	signer := auth.NewSigner("test-secret", time.Hour)
	h := handler.New(svc, signer, st)
	r := router.New(h, signer)
	return &apiEnv{r: r, tokens: map[string]string{}}
}

func (e *apiEnv) do(t *testing.T, method, path, token string, body any) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	e.r.ServeHTTP(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func (e *apiEnv) login(t *testing.T, user, pwd string) string {
	t.Helper()
	code, out := e.do(t, http.MethodPost, "/api/v1/auth/login", "", map[string]string{
		"username": user, "password": pwd,
	})
	if code != http.StatusOK {
		t.Fatalf("login %s failed: %d %v", user, code, out)
	}
	tok, _ := out["token"].(string)
	if tok == "" {
		t.Fatalf("login %s no token", user)
	}
	return tok
}

// TestRBACEnforcement 5 角色权限矩阵越权拦截（AC-23）。
func TestRBACEnforcement(t *testing.T) {
	e := newAPI(t)
	proj := e.login(t, "project", "Proj@123")
	wh := e.login(t, "warehouse", "Wh@123")
	qc := e.login(t, "inspector", "Insp@123")
	admin := e.login(t, "admin", "Admin@123")

	// 客户角色：一期不开放（登录应 403）
	code, _ := e.do(t, http.MethodPost, "/api/v1/auth/login", "", map[string]string{
		"username": "customer", "password": "Cust@123",
	})
	if code != http.StatusForbidden {
		t.Fatalf("客户登录应 403，实际 %d", code)
	}

	// 仓库操作员不可录入预报（仅项目人员）
	code, _ = e.do(t, http.MethodPost, "/api/v1/forecasts", wh, map[string]any{
		"rma_no": "RMA-RBAC", "pn_code": "PN-A100", "customer": "BOE US",
		"sn_list": []string{"SN-RBAC-1"},
	})
	if code != http.StatusForbidden {
		t.Fatalf("仓库录入预报应 403，实际 %d", code)
	}

	// 质检员不可审批差异
	code, _ = e.do(t, http.MethodPost, "/api/v1/discrepancies/D0001/approve", qc, map[string]any{"note": "x"})
	if code != http.StatusForbidden {
		t.Fatalf("质检审批差异应 403，实际 %d", code)
	}

	// 项目人员不可执行 kitting 扫码
	code, _ = e.do(t, http.MethodPost, "/api/v1/boxes", proj, map[string]any{"batch_no": "X"})
	if code != http.StatusForbidden {
		t.Fatalf("项目人员扫码应 403，实际 %d", code)
	}

	// 管理员无仓库作业权限
	code, _ = e.do(t, http.MethodGet, "/api/v1/audit-logs", admin, nil)
	if code != http.StatusOK {
		t.Fatalf("管理员查审计应 200，实际 %d", code)
	}

	// 未带 token
	code, _ = e.do(t, http.MethodGet, "/api/v1/forecasts", "", nil)
	if code != http.StatusUnauthorized {
		t.Fatalf("无 token 应 401，实际 %d", code)
	}
}

// TestEndToEndSmoke 主流程冒烟：预报→允收→kitting→质检→还货→发运→RMA自动关闭。
func TestEndToEndSmoke(t *testing.T) {
	e := newAPI(t)
	proj := e.login(t, "project", "Proj@123")
	wh := e.login(t, "warehouse", "Wh@123")
	qc := e.login(t, "inspector", "Insp@123")

	sns := []string{"SN-E2E-1", "SN-E2E-2"}
	code, fo := e.do(t, http.MethodPost, "/api/v1/forecasts", proj, map[string]any{
		"rma_no": "RMA-E2E", "pn_code": "PN-A100", "model_code": "MD-55", "customer": "BOE US",
		"sn_list": sns,
	})
	if code != http.StatusCreated {
		t.Fatalf("create forecast: %d %v", code, fo)
	}
	forecastNo, _ := fo["forecast_no"].(string)

	code, batch := e.do(t, http.MethodPost, "/api/v1/receiving/batches", wh, map[string]any{
		"forecast_no": forecastNo, "label_qty_total": 2, "pod_photo_ids": []string{"POD-1"},
	})
	if code != http.StatusCreated {
		t.Fatalf("create batch: %d %v", code, batch)
	}
	batchNo, _ := batch["batch_no"].(string)

	code, box := e.do(t, http.MethodPost, "/api/v1/boxes", wh, map[string]any{"batch_no": batchNo})
	if code != http.StatusCreated {
		t.Fatalf("create box: %d %v", code, box)
	}
	boxNo, _ := box["box_no"].(string)
	if len(boxNo) != 12 || boxNo[:3] != "BOE" {
		t.Fatalf("箱号格式错误: %s", boxNo)
	}

	for _, sn := range sns {
		code, res := e.do(t, http.MethodPost, "/api/v1/boxes/scan", wh, map[string]any{
			"box_no": boxNo, "sn": sn,
		})
		if code != http.StatusOK || res["result"] != "OK" {
			t.Fatalf("scan %s: %d %v", sn, code, res)
		}
	}
	code, _ = e.do(t, http.MethodPost, "/api/v1/boxes/"+boxNo+"/seal", wh, map[string]any{})
	if code != http.StatusOK {
		t.Fatalf("seal: %d", code)
	}

	// 箱标（HTML）
	req := httptest.NewRequest(http.MethodGet, "/api/v1/boxes/"+boxNo+"/label", nil)
	req.Header.Set("Authorization", "Bearer "+wh)
	w := httptest.NewRecorder()
	e.r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !bytes.Contains(w.Body.Bytes(), []byte("SN-E2E-1")) {
		t.Fatalf("label html: %d", w.Code)
	}

	// 批量质检判定
	code, judged := e.do(t, http.MethodPost, "/api/v1/inspections/batch", qc, map[string]any{
		"items": []map[string]any{
			{"sn": sns[0], "judgment_result": "REPAIR"},
			{"sn": sns[1], "judgment_result": "REPAIR"},
		},
	})
	if code != http.StatusOK {
		t.Fatalf("judge: %d %v", code, judged)
	}

	// 还货 + 发运
	for i, sn := range sns {
		code, plan := e.do(t, http.MethodPost, "/api/v1/returns", wh, map[string]any{"original_sn": sn})
		if code != http.StatusCreated {
			t.Fatalf("return %s: %d %v", sn, code, plan)
		}
		ob, _ := plan["return_outbound"].(map[string]any)
		obNo, _ := ob["outbound_no"].(string)
		code, sh := e.do(t, http.MethodPost, "/api/v1/outbounds/"+obNo+"/ship", wh, map[string]any{
			"carrier": "DHL", "tracking_no": fmt.Sprintf("TRK-%d", i),
		})
		if code != http.StatusOK {
			t.Fatalf("ship %s: %d %v", sn, code, sh)
		}
	}

	// RMA 自动关闭
	code, prog := e.do(t, http.MethodGet, "/api/v1/rma/"+forecastNo+"/return-progress", proj, nil)
	if code != http.StatusOK {
		t.Fatalf("progress: %d %v", code, prog)
	}
	if prog["rma_status"] != "CLOSED" {
		t.Fatalf("期望 RMA CLOSED，实际 %v", prog["rma_status"])
	}
	if prog["returned"].(float64) != 2 {
		t.Fatalf("期望已还货 2，实际 %v", prog["returned"])
	}
}

// TestShipRequiresLogistics 发运必须提交物流公司与单号（OB-09）。
func TestShipRequiresLogistics(t *testing.T) {
	e := newAPI(t)
	proj := e.login(t, "project", "Proj@123")
	wh := e.login(t, "warehouse", "Wh@123")
	qc := e.login(t, "inspector", "Insp@123")

	_, fo := e.do(t, http.MethodPost, "/api/v1/forecasts", proj, map[string]any{
		"rma_no": "RMA-LG", "pn_code": "PN-A100", "customer": "BOE US",
		"sn_list": []string{"SN-LG-1"},
	})
	forecastNo := fo["forecast_no"].(string)
	_, batch := e.do(t, http.MethodPost, "/api/v1/receiving/batches", wh, map[string]any{
		"forecast_no": forecastNo, "label_qty_total": 1, "pod_photo_ids": []string{"POD"},
	})
	_, box := e.do(t, http.MethodPost, "/api/v1/boxes", wh, map[string]any{"batch_no": batch["batch_no"]})
	e.do(t, http.MethodPost, "/api/v1/boxes/scan", wh, map[string]any{
		"box_no": box["box_no"], "sn": "SN-LG-1",
	})
	e.do(t, http.MethodPost, "/api/v1/boxes/"+box["box_no"].(string)+"/seal", wh, map[string]any{})
	e.do(t, http.MethodPost, "/api/v1/inspections", qc, map[string]any{
		"sn": "SN-LG-1", "judgment_result": "REPAIR",
	})
	_, plan := e.do(t, http.MethodPost, "/api/v1/returns", wh, map[string]any{"original_sn": "SN-LG-1"})
	ob := plan["return_outbound"].(map[string]any)["outbound_no"].(string)
	code, out := e.do(t, http.MethodPost, "/api/v1/outbounds/"+ob+"/ship", wh, map[string]any{"carrier": "DHL"})
	if code != http.StatusBadRequest {
		t.Fatalf("缺物流单号应 400，实际 %d %v", code, out)
	}
}

var _ = gorm.ErrRecordNotFound
