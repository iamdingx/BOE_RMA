// Package middleware JWT 鉴权 + 5 角色 RBAC + 统一错误响应。
package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/boe/rma/internal/auth"
	"github.com/boe/rma/internal/enum"
	"github.com/boe/rma/internal/errs"
)

const CtxClaims = "claims"

// 权限键（与 PRD 第 3 章权限矩阵一一对应）
const (
	PermMasterWrite    = "master.write"      // 主数据维护/导入：项目/管理员
	PermUserManage     = "user.manage"       // 用户角色管理：管理员
	PermForecastWrite  = "forecast.write"    // 预报录入：项目
	PermForecastFix    = "forecast.correct"  // 到货预报修正（受控）：项目
	PermReceiving      = "receiving.write"   // 物流允收：仓库
	PermKitting        = "kitting.write"     // 逐片扫码：仓库
	PermDiscSubmit     = "discrepancy.submit"// 差异现场提交：仓库
	PermDiscApprove    = "discrepancy.approve" // 差异审批：项目
	PermCorrection     = "correction.write"  // 受控修正：项目
	PermInspect        = "inspection.write"  // 质检判定：质检
	PermCIDConfirm     = "cid.confirm"       // CID 客户确认：项目
	PermOutbound       = "outbound.write"    // 出库扫描/单创建：仓库
	PermScrapApprove   = "outbound.scrap.approve" // 报废审批：项目
	PermBufferPlan     = "buffer.inbound.plan"    // 备件入库预报：项目
	PermBufferExec     = "buffer.inbound.exec"    // 备件入库扫码：仓库
	PermLabelPrint     = "label.print"       // 箱标打印：仓库
	PermPacking        = "packing.write"     // 打包发运：仓库
	PermRetained       = "retained.dispose"  // RETAINED 处置：项目
	PermTATView        = "tat.view"          // TAT 看板：全角色
	PermReportView     = "report.view"       // 报表：全角色
	PermAnomalyView    = "anomaly.view"      // 双异常查看：全角色
	PermAuditView      = "audit.view"        // 审计日志：项目/管理员
)

// rolePerms 角色 → 权限集合（PRD 第 3 章矩阵固化）。
var rolePerms = map[enum.Role]map[string]bool{
	enum.RoleWarehouse: {
		PermReceiving: true, PermKitting: true, PermDiscSubmit: true,
		PermOutbound: true, PermBufferExec: true, PermLabelPrint: true, PermPacking: true,
		PermTATView: true, PermReportView: true, PermAnomalyView: true,
	},
	enum.RoleProject: {
		PermMasterWrite: true, PermForecastWrite: true, PermForecastFix: true,
		PermDiscApprove: true, PermCorrection: true, PermCIDConfirm: true,
		PermScrapApprove: true, PermBufferPlan: true, PermRetained: true,
		PermTATView: true, PermReportView: true, PermAnomalyView: true, PermAuditView: true,
	},
	enum.RoleInspector: {
		PermInspect: true,
		PermTATView: true, PermReportView: true, PermAnomalyView: true,
	},
	// 客户一期不开放访问（C-09）：无任何业务权限，登录直接 403
	enum.RoleCustomer: {},
	enum.RoleAdmin: {
		PermMasterWrite: true, PermUserManage: true,
		PermTATView: true, PermReportView: true, PermAnomalyView: true, PermAuditView: true,
	},
}

// HasPerm 判定角色是否具备权限。
func HasPerm(role enum.Role, perm string) bool {
	if perm == "" {
		return true
	}
	return rolePerms[role][perm]
}

// Auth 解析 Bearer token 并注入 claims。
func Auth(signer *auth.Signer) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": "UNAUTHORIZED", "message": "缺少认证 token"})
			return
		}
		claims, err := signer.Parse(strings.TrimPrefix(h, "Bearer "))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": "UNAUTHORIZED", "message": "token 无效或已过期"})
			return
		}
		c.Set(CtxClaims, claims)
		c.Next()
	}
}

// Require 权限校验（越权拦截）。
func Require(perm string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, ok := Claims(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": "UNAUTHORIZED", "message": "未认证"})
			return
		}
		role := enum.Role(claims.Role)
		if role == enum.RoleCustomer {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": "CUSTOMER_DISABLED", "message": "客户账号一期不开放系统访问"})
			return
		}
		if !HasPerm(role, perm) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": "FORBIDDEN", "message": "当前角色无此操作权限"})
			return
		}
		c.Next()
	}
}

// Claims 取当前用户 claims。
func Claims(c *gin.Context) (*auth.Claims, bool) {
	v, ok := c.Get(CtxClaims)
	if !ok {
		return nil, false
	}
	cl, ok := v.(*auth.Claims)
	return cl, ok
}

// Operator 当前操作人用户名。
func Operator(c *gin.Context) string {
	if cl, ok := Claims(c); ok {
		return cl.Username
	}
	return ""
}

// Fail 统一错误响应（AppError → HTTP）。
func Fail(c *gin.Context, err error) {
	var ae *errs.AppError
	if errors.As(err, &ae) {
		c.JSON(ae.HTTP, gin.H{"code": ae.Code, "message": ae.Msg})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"code": "INTERNAL", "message": "服务器内部错误"})
}

// Recovery 捕获 panic。
func Recovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, rec any) {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"code": "PANIC", "message": "服务异常，已回滚事务"})
	})
}
