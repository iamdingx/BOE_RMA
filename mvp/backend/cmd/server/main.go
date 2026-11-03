// BOE US RMA 收发管理系统 — 一期 MVP 服务入口。
package main

import (
	"log"
	"os"
	"time"

	"github.com/boe/rma/internal/auth"
	"github.com/boe/rma/internal/handler"
	"github.com/boe/rma/internal/router"
	"github.com/boe/rma/internal/service"
	"github.com/boe/rma/internal/storage"
	"github.com/boe/rma/internal/store"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	port := env("APP_PORT", "8080")
	dialect := env("DB_DIALECT", "sqlite")
	dsn := env("DB_DSN", "rma.db")
	tenant := env("TENANT_ID", "BOE_US")
	jwtSecret := env("JWT_SECRET", "boe-rma-dev-secret-change-in-prod")

	db, err := store.Open(dialect, dsn)
	if err != nil {
		log.Fatalf("数据库连接失败: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		log.Fatalf("数据库迁移失败: %v", err)
	}
	if err := store.Seed(db); err != nil {
		log.Fatalf("初始化数据失败: %v", err)
	}

	var st storage.Storage
	if ep := os.Getenv("MINIO_ENDPOINT"); ep != "" {
		m, err := storage.NewMinIO(
			ep, env("MINIO_ACCESS_KEY", "minioadmin"), env("MINIO_SECRET_KEY", "minioadmin"),
			env("MINIO_BUCKET", "rma-photos"), os.Getenv("MINIO_USE_SSL") == "true",
			env("MINIO_PUBLIC_BASE", ""),
		)
		if err != nil {
			log.Fatalf("对象存储初始化失败: %v", err)
		}
		st = m
		log.Println("[storage] MinIO 已启用")
	} else {
		st = storage.NewMem()
		log.Println("[storage] 未配置 MINIO_ENDPOINT，使用内存对象存储（仅开发）")
	}

	svc := service.New(db, tenant, st)
	signer := auth.NewSigner(jwtSecret, 12*time.Hour)
	h := handler.New(svc, signer, st)
	r := router.New(h, signer)

	log.Printf("BOE US RMA MVP 启动，端口 :%s（dialect=%s tenant=%s）", port, dialect, tenant)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}
