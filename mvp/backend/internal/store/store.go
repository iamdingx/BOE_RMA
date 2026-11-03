// Package store 数据库连接、迁移与初始化数据。
package store

import (
	"fmt"
	"time"

	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/boe/rma/internal/enum"
	"github.com/boe/rma/internal/model"
)

// Open 打开数据库。dialect: mysql | sqlite。
func Open(dialect, dsn string) (*gorm.DB, error) {
	cfg := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}
	switch dialect {
	case "mysql":
		return gorm.Open(mysql.Open(dsn), cfg)
	case "sqlite":
		return gorm.Open(sqlite.Open(dsn), cfg)
	default:
		return nil, fmt.Errorf("unsupported dialect: %s", dialect)
	}
}

// Migrate 自动建表（生产环境以 db/schema.sql 为准，此处用于开发/测试）。
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(model.AllModels()...)
}

// Seed 初始化租户、主数据与 5 角色账号（幂等）。
func Seed(db *gorm.DB) error {
	const tenant = "BOE_US"

	type seedUser struct {
		ID, Name, Pwd, Display string
		Role                   enum.Role
	}
	users := []seedUser{
		{"U-ADMIN", "admin", "Admin@123", "系统管理员", enum.RoleAdmin},
		{"U-PROJ", "project", "Proj@123", "项目人员", enum.RoleProject},
		{"U-WH", "warehouse", "Wh@123", "仓库操作员", enum.RoleWarehouse},
		{"U-QC", "inspector", "Insp@123", "质检员/R&D", enum.RoleInspector},
		{"U-CUST", "customer", "Cust@123", "客户/BOE US（一期不开放）", enum.RoleCustomer},
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ?", tenant).First(&model.Tenant{}).Error; err != nil {
			if err := tx.Create(&model.Tenant{TenantID: tenant, TenantName: "BOE US"}).Error; err != nil {
				return err
			}
		}
		for _, u := range users {
			var cnt int64
			tx.Model(&model.User{}).Where("tenant_id = ? AND username = ?", tenant, u.Name).Count(&cnt)
			if cnt > 0 {
				continue
			}
			hash, err := bcrypt.GenerateFromPassword([]byte(u.Pwd), bcrypt.DefaultCost)
			if err != nil {
				return err
			}
			if err := tx.Create(&model.User{
				TenantID: tenant, UserID: u.ID, Username: u.Name,
				PasswordHash: string(hash), DisplayName: u.Display, Role: string(u.Role), Enabled: true,
			}).Error; err != nil {
				return err
			}
		}
		// 主数据样例
		models := []model.ProductModel{
			{TenantID: tenant, ModelCode: "MD-55", ModelName: "55寸商用显示模组", CreatedAt: time.Now()},
			{TenantID: tenant, ModelCode: "MD-65", ModelName: "65寸商用显示模组", CreatedAt: time.Now()},
		}
		for _, m := range models {
			var c int64
			tx.Model(&model.ProductModel{}).Where("tenant_id=? AND model_code=?", tenant, m.ModelCode).Count(&c)
			if c == 0 {
				if err := tx.Create(&m).Error; err != nil {
					return err
				}
			}
		}
		pns := []model.PartNumber{
			{TenantID: tenant, PNCode: "PN-A100", ModelCode: "MD-55", Description: "55寸主板", CreatedAt: time.Now()},
			{TenantID: tenant, PNCode: "PN-B200", ModelCode: "MD-65", Description: "65寸主板", CreatedAt: time.Now()},
		}
		for _, p := range pns {
			var c int64
			tx.Model(&model.PartNumber{}).Where("tenant_id=? AND pn_code=?", tenant, p.PNCode).Count(&c)
			if c == 0 {
				if err := tx.Create(&p).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}
