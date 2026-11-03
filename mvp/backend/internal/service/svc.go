// Package service 业务编排层：所有状态机校验、事务、审计的唯一入口。
package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/boe/rma/internal/enum"
	"github.com/boe/rma/internal/errs"
	"github.com/boe/rma/internal/model"
	"github.com/boe/rma/internal/statemachine"
	"github.com/boe/rma/internal/storage"
)

// Service 业务服务。
type Service struct {
	DB     *gorm.DB
	Tenant string
	Now    func() time.Time
	Store  storage.Storage
}

func New(db *gorm.DB, tenant string, st storage.Storage) *Service {
	return &Service{DB: db, Tenant: tenant, Now: time.Now, Store: st}
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Tx 事务包装：任何 panic/error 自动回滚。
func (s *Service) Tx(fn func(tx *gorm.DB) error) error {
	return s.DB.Transaction(fn)
}

// lock 行级锁（MySQL 生效；SQLite 串行写天然隔离）。
func lock(tx *gorm.DB) *gorm.DB {
	if tx.Dialector.Name() == "mysql" {
		return tx.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	return tx
}

// audit 写审计日志（只增不删，M-14）。
func (s *Service) audit(tx *gorm.DB, action enum.AuditAction, bizType, bizNo, field, oldVal, newVal, reason, operator, approvedBy string) error {
	return tx.Create(&model.AuditLog{
		TenantID: s.Tenant, BizType: bizType, BizNo: bizNo, Action: string(action),
		Field: field, OldValue: oldVal, NewValue: newVal, Reason: reason,
		Operator: operator, ApprovedBy: approvedBy, CreatedAt: s.now(),
	}).Error
}

// ---------- 通用查询 ----------

func (s *Service) getForecast(tx *gorm.DB, forecastNo string) (*model.ForecastOrder, error) {
	var fo model.ForecastOrder
	if err := tx.Where("tenant_id = ? AND forecast_no = ?", s.Tenant, forecastNo).First(&fo).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.NotFound("预报单 " + forecastNo)
		}
		return nil, err
	}
	return &fo, nil
}

func (s *Service) getSN(tx *gorm.DB, sn string) (*model.SNItem, error) {
	var it model.SNItem
	q := lock(tx).Where("tenant_id = ? AND sn = ?", s.Tenant, sn).First(&it)
	if q.Error != nil {
		if errors.Is(q.Error, gorm.ErrRecordNotFound) {
			return nil, errs.NotFound("SN " + sn)
		}
		return nil, q.Error
	}
	return &it, nil
}

// transit 执行 SN 状态跃迁（唯一入口，含状态机校验与时间戳落库）。
func (s *Service) transit(tx *gorm.DB, sn string, to enum.SNStatus, ctx statemachine.SNContext, operator string) (*model.SNItem, error) {
	it, err := s.getSN(tx, sn)
	if err != nil {
		return nil, err
	}
	from := enum.SNStatus(it.Status)
	if from == to {
		return it, nil
	}
	if err := statemachine.CanSNTransit(from, to, ctx); err != nil {
		return nil, err
	}
	now := s.now()
	updates := map[string]any{"status": string(to)}
	switch to {
	case enum.SNReceived:
		updates["received_at"] = now
		updates["locked"] = true
	case enum.SNInspecting:
		updates["inspected_at"] = now
	case enum.SNJudged:
		updates["judged_at"] = now
	case enum.SNOutbound:
		updates["outbound_at"] = now
	case enum.SNCompleted, enum.SNDisposed:
		updates["completed_at"] = now
	case enum.SNRetained:
		// 留库待定：不落 completed_at
	}
	if err := tx.Model(&model.SNItem{}).Where("tenant_id = ? AND sn = ?", s.Tenant, sn).
		Updates(updates).Error; err != nil {
		return nil, err
	}
	it.Status = string(to)
	if err := s.audit(tx, enum.AuditStatusTransition, "sn_item", sn, "status",
		string(from), string(to), "状态机流转", operator, ""); err != nil {
		return nil, err
	}
	_ = now
	return it, nil
}

// setSNFields 更新 SN 业务字段（不涉及状态）。
func (s *Service) setSNFields(tx *gorm.DB, sn string, updates map[string]any) error {
	return tx.Model(&model.SNItem{}).Where("tenant_id = ? AND sn = ?", s.Tenant, sn).Updates(updates).Error
}

// trimAll 归一化 SN/PN（扫码枪常带空白/回车）。
func trimAll(v string) string { return strings.ToUpper(strings.TrimSpace(v)) }

func normSN(v string) string { return trimAll(v) }

// paged 通用分页（默认 20，允许 10/20/50/200）。
func paged(db *gorm.DB, page, size int) (*gorm.DB, int, int) {
	if page < 1 {
		page = 1
	}
	switch size {
	case 10, 20, 50, 200:
	default:
		size = 20
	}
	return db.Offset((page - 1) * size).Limit(size), page, size
}

// PageResult 统一分页返回体。
type PageResult struct {
	Items any   `json:"items"`
	Total int64 `json:"total"`
	Page  int   `json:"page"`
	Size  int   `json:"size"`
}

func wrapPage(items any, total int64, page, size int) PageResult {
	return PageResult{Items: items, Total: total, Page: page, Size: size}
}

// ptrTime 便捷时间指针。
func ptrTime(t time.Time) *time.Time { return &t }

// findOrErr 通用首行查询。
func (s *Service) findOrErr(tx *gorm.DB, out any, what string, query string, args ...any) error {
	if err := tx.Where(query, args...).First(out).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errs.NotFound(what)
		}
		return err
	}
	return nil
}

func itoa(i int) string { return fmt.Sprintf("%d", i) }
