// Package router 路由注册 + 权限绑定（每条路由显式声明所需权限）。
package router

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/boe/rma/internal/auth"
	"github.com/boe/rma/internal/handler"
	"github.com/boe/rma/internal/middleware"
)

// New 构建 gin 引擎。
func New(h *handler.Handler, signer *auth.Signer) *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), middleware.Recovery())
	r.Use(cors())

	r.GET("/health", handler.Health)
	r.POST("/api/v1/auth/login", h.Login)

	api := r.Group("/api/v1")
	api.Use(middleware.Auth(signer))
	{
		api.GET("/auth/me", h.Me)

		// 主数据（MD-01/02/03）
		api.GET("/master/models", middleware.Require(middleware.PermReportView), h.ListModels)
		api.POST("/master/models", middleware.Require(middleware.PermMasterWrite), h.CreateModel)
		api.GET("/master/pns", middleware.Require(middleware.PermReportView), h.ListPNs)
		api.POST("/master/pns", middleware.Require(middleware.PermMasterWrite), h.CreatePN)
		api.POST("/master/import", middleware.Require(middleware.PermMasterWrite), h.ImportMaster)
		api.GET("/users", middleware.Require(middleware.PermUserManage), h.ListUsers)
		api.POST("/users", middleware.Require(middleware.PermUserManage), h.CreateUser)

		// 照片
		api.POST("/photos", middleware.Require(middleware.PermKitting), h.UploadPhoto)
		api.GET("/photos", middleware.Require(middleware.PermAnomalyView), h.ListPhotos)

		// 预报（FC-01~08）
		api.POST("/forecasts", middleware.Require(middleware.PermForecastWrite), h.CreateForecast)
		api.GET("/forecasts", middleware.Require(middleware.PermReportView), h.ListForecast)
		api.GET("/forecasts/:no", middleware.Require(middleware.PermReportView), h.GetForecast)
		api.GET("/forecasts/:no/export", middleware.Require(middleware.PermReportView), h.ExportForecast)
		api.POST("/forecasts/import-sn", middleware.Require(middleware.PermForecastWrite), h.ImportForecastSN)
		api.GET("/forecasts/template", middleware.Require(middleware.PermForecastWrite), h.ForecastSNTemplate)

		// 物流允收（RC-01~08）
		api.POST("/receiving/batches", middleware.Require(middleware.PermReceiving), h.CreateBatch)
		api.GET("/receiving/batches", middleware.Require(middleware.PermReportView), h.ListBatches)

		// 收货箱 / kitting（KT-01~08）
		api.POST("/boxes", middleware.Require(middleware.PermKitting), h.CreateBox)
		api.GET("/boxes", middleware.Require(middleware.PermReportView), h.ListBoxes)
		api.GET("/boxes/:no", middleware.Require(middleware.PermReportView), h.BoxDetail)
		api.POST("/boxes/scan", middleware.Require(middleware.PermKitting), h.Scan)
		api.POST("/boxes/:no/seal", middleware.Require(middleware.PermKitting), h.SealBox)
		api.GET("/boxes/:no/label", middleware.Require(middleware.PermLabelPrint), h.ReceiveLabel)

		// 差异（DC-01~11）
		api.GET("/discrepancies", middleware.Require(middleware.PermAnomalyView), h.ListDiscrepancies)
		api.GET("/discrepancies/:no", middleware.Require(middleware.PermAnomalyView), h.GetDiscrepancy)
		api.POST("/discrepancies/:no/submit", middleware.Require(middleware.PermDiscSubmit), h.SubmitDiscrepancy)
		api.POST("/discrepancies/:no/approve", middleware.Require(middleware.PermDiscApprove), h.ApproveDiscrepancy)
		api.POST("/discrepancies/:no/reject", middleware.Require(middleware.PermDiscApprove), h.RejectDiscrepancy)

		// 质检（IN-01~04）
		api.GET("/inspections", middleware.Require(middleware.PermInspect), h.ListInspection)
		api.POST("/inspections", middleware.Require(middleware.PermInspect), h.Judge)
		api.POST("/inspections/batch", middleware.Require(middleware.PermInspect), h.JudgeBatch)

		// 出库（OB-01~11）
		api.POST("/outbounds", middleware.Require(middleware.PermOutbound), h.CreateOutbound)
		api.GET("/outbounds", middleware.Require(middleware.PermReportView), h.ListOutbounds)
		api.GET("/outbounds/:no", middleware.Require(middleware.PermReportView), h.OutboundDetail)
		api.POST("/outbounds/:no/ship", middleware.Require(middleware.PermOutbound), h.ShipOutbound)
		api.POST("/outbounds/:no/count-confirm", middleware.Require(middleware.PermOutbound), h.ConfirmCount)
		api.POST("/outbounds/:no/scrap-approve", middleware.Require(middleware.PermScrapApprove), h.ApproveScrap)
		api.POST("/outbounds/:no/scrap-reject", middleware.Require(middleware.PermScrapApprove), h.RejectScrap)
		api.POST("/outbounds/:no/scrap-resubmit", middleware.Require(middleware.PermOutbound), h.ResubmitScrap)

		// 打包 / 发运（OB-07~10）
		api.POST("/shipment-boxes", middleware.Require(middleware.PermPacking), h.CreateShipmentBox)
		api.GET("/shipment-boxes", middleware.Require(middleware.PermReportView), h.ListShipmentBoxes)
		api.POST("/shipment-boxes/:no/items", middleware.Require(middleware.PermPacking), h.AddShipmentItem)
		api.POST("/shipment-boxes/:no/seal", middleware.Require(middleware.PermPacking), h.SealShipmentBox)
		api.GET("/shipment-boxes/:no/label", middleware.Require(middleware.PermLabelPrint), h.ShipmentLabel)

		// 还货（RT-01~09）
		api.POST("/returns", middleware.Require(middleware.PermOutbound), h.CreateReturn)
		api.GET("/rma/:no/return-progress", middleware.Require(middleware.PermTATView), h.ReturnProgress)
		api.GET("/return-board", middleware.Require(middleware.PermTATView), h.ReturnBoard)
		api.POST("/retained/:sn/dispose", middleware.Require(middleware.PermRetained), h.DisposeRetained)

		// BUFFER（BS-01~09）
		api.POST("/buffer/inbound", middleware.Require(middleware.PermBufferExec), h.BufferInbound)
		api.GET("/buffer/stock", middleware.Require(middleware.PermReportView), h.BufferStock)
		api.GET("/buffer/sns", middleware.Require(middleware.PermReportView), h.BufferSNList)
		api.POST("/buffer/:sn/scrap", middleware.Require(middleware.PermBufferPlan), h.ScrapBuffer)
		api.POST("/buffer/substitute", middleware.Require(middleware.PermOutbound), h.RequestSubstitute)
		api.POST("/buffer/substitute/:no/approve", middleware.Require(middleware.PermBufferPlan), h.ApproveSubstitute)

		// 报表（RP-01~06）
		api.GET("/reports/rma", middleware.Require(middleware.PermReportView), h.ReportRMA)
		api.GET("/reports/sku", middleware.Require(middleware.PermReportView), h.ReportSKU)
		api.GET("/reports/trend", middleware.Require(middleware.PermReportView), h.ReportTrend)
		api.GET("/reports/discrepancy", middleware.Require(middleware.PermReportView), h.ReportDiscrepancy)
		api.GET("/reports/detail", middleware.Require(middleware.PermReportView), h.ReportDetail)
		api.GET("/reports/tat", middleware.Require(middleware.PermTATView), h.ReportTAT)
		api.GET("/reports/tat-alerts", middleware.Require(middleware.PermTATView), h.TATAlerts)
		api.POST("/reports/tat-refresh", middleware.Require(middleware.PermTATView), h.RefreshTAT)

		// SN 轨迹 / 审计
		api.GET("/sn/:sn/trace", middleware.Require(middleware.PermReportView), h.SNTrace)
		api.GET("/audit-logs", middleware.Require(middleware.PermAuditView), h.ListAuditLogs)
	}
	return r
}

func cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Authorization,Content-Type")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
