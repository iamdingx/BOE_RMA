package service

import (
	"math"
	"time"

	"gorm.io/gorm"

	"github.com/boe/rma/internal/enum"
	"github.com/boe/rma/internal/errs"
	"github.com/boe/rma/internal/model"
)

// TAT 常量（RT-07/08）：≤10 自然日；第 8 天预警；>10 标红。
const (
	TATLimitDays   = 10
	TATWarningDays = 8
)

// ReconcileRMA RMA 状态聚合推进（V1.13 口径）+ 自动关闭 + TAT 落库。
func (s *Service) ReconcileRMA(forecastNo, operator string) error {
	if forecastNo == "" {
		return nil
	}
	return s.Tx(func(tx *gorm.DB) error {
		fo, err := s.getForecast(tx, forecastNo)
		if err != nil {
			return err
		}
		if enum.RMAStatus(fo.RMAStatus) == enum.RMAClosed {
			return nil
		}

		var sns []model.SNItem
		if err := tx.Where("tenant_id = ? AND forecast_no = ?", s.Tenant, forecastNo).Find(&sns).Error; err != nil {
			return err
		}
		arrived, closedShort, judgedDone, pendingShortDisc := 0, 0, 0, int64(0)
		for _, it := range sns {
			st := enum.SNStatus(it.Status)
			switch st {
			case enum.SNClosedShort:
				closedShort++
				judgedDone++
			case enum.SNForecasted:
				// 未到货
			default:
				arrived++
				if st != enum.SNReceived && st != enum.SNDiscrepancy && st != enum.SNInspecting {
					judgedDone++
				}
			}
		}
		if err := tx.Model(&model.DiscrepancyRecord{}).
			Where("tenant_id = ? AND forecast_no = ? AND discrepancy_type = ? AND approval_status = ?",
				s.Tenant, forecastNo, string(enum.DiscQtyShort), string(enum.ApprovalPending)).
			Count(&pendingShortDisc).Error; err != nil {
			return err
		}

		// 到齐判定（M-06 / RK-01）：已收 + CLOSED_SHORT ≥ 预报量 且 少收审核流已闭环
		arrivedFull := arrived+closedShort >= fo.ForecastQty && pendingShortDisc == 0 && fo.ForecastQty > 0

		// 逐级推进（禁止跳级）
		for {
			cur := enum.RMAStatus(fo.RMAStatus)
			var next enum.RMAStatus
			switch cur {
			case enum.RMAOpen:
				if arrived > 0 || closedShort > 0 {
					next = enum.RMAReceiving
				}
			case enum.RMAReceiving:
				if arrivedFull {
					next = enum.RMAProcessing
				}
			case enum.RMAProcessing:
				if len(sns) > 0 && judgedDone == len(sns) {
					next = enum.RMAPendingReturn
				}
			case enum.RMAPendingReturn:
				done, total, err := s.returnProgress(tx, fo)
				if err != nil {
					return err
				}
				if total > 0 && done >= total {
					next = enum.RMAReturned
				}
			case enum.RMAReturned:
				next = enum.RMAClosed
			}
			if next == "" {
				break
			}
			if err := s.advanceRMA(tx, fo, next, operator); err != nil {
				return err
			}
			if next == enum.RMAClosed {
				if err := s.closeRMA(tx, fo, operator); err != nil {
					return err
				}
				break
			}
		}
		// 更新 TAT 标记
		return s.updateTATFlags(tx, fo)
	})
}

// returnProgress 计算还货进度（RK-13：按 return_record 计数；分母排除 CLOSED_SHORT）。
func (s *Service) returnProgress(tx *gorm.DB, fo *model.ForecastOrder) (done int, total int, err error) {
	var eligible []string
	if err := tx.Model(&model.SNItem{}).
		Where("tenant_id = ? AND forecast_no = ? AND status <> ?", s.Tenant, fo.ForecastNo, string(enum.SNClosedShort)).
		Pluck("sn", &eligible).Error; err != nil {
		return 0, 0, err
	}
	if len(eligible) == 0 {
		return 0, 0, nil
	}
	var cnt int64
	if err := tx.Model(&model.ReturnRecord{}).
		Where("tenant_id = ? AND forecast_no = ? AND status = ? AND original_sn IN ?",
			s.Tenant, fo.ForecastNo, "SHIPPED", eligible).
		Count(&cnt).Error; err != nil {
		return 0, 0, err
	}
	return int(cnt), len(eligible), nil
}

// closeRMA 关闭 RMA：落 closed_at + TAT（C-05 同一时点）。
func (s *Service) closeRMA(tx *gorm.DB, fo *model.ForecastOrder, operator string) error {
	now := s.now()
	updates := map[string]any{"closed_at": now}
	if fo.FullReceivedAt != nil {
		d := TatDays(*fo.FullReceivedAt, now)
		updates["tat_days"] = d
		updates["tat_warning"] = d >= TATWarningDays
		updates["tat_overdue"] = d > TATLimitDays
	}
	if err := tx.Model(&model.ForecastOrder{}).
		Where("tenant_id = ? AND forecast_no = ?", s.Tenant, fo.ForecastNo).
		Updates(updates).Error; err != nil {
		return err
	}
	return s.audit(tx, enum.AuditRMAClose, "forecast_order", fo.ForecastNo, "rma_status",
		string(enum.RMAReturned), string(enum.RMAClosed),
		"系统自动关闭（100% 还货）", operator, "")
}

// TatDays TAT 自然日 = ceil((end-start)/24h)（R-007 口径）。
func TatDays(start, end time.Time) int {
	if end.Before(start) {
		return 0
	}
	h := end.Sub(start).Hours()
	return int(math.Ceil(h / 24.0))
}

// updateTATFlags 对未关闭 RMA 按当前时间计算预警/超期标记。
func (s *Service) updateTATFlags(tx *gorm.DB, fo *model.ForecastOrder) error {
	if fo.FullReceivedAt == nil || fo.ClosedAt != nil {
		return nil
	}
	d := TatDays(*fo.FullReceivedAt, s.now())
	warn := d >= TATWarningDays
	overdue := d > TATLimitDays
	if boolToInt(warn) == boolToInt(fo.TatWarning) && boolToInt(overdue) == boolToInt(fo.TatOverdue) {
		return nil
	}
	return tx.Model(&model.ForecastOrder{}).
		Where("tenant_id = ? AND forecast_no = ?", s.Tenant, fo.ForecastNo).
		Updates(map[string]any{"tat_warning": warn, "tat_overdue": overdue}).Error
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// RefreshTAT 批量刷新 TAT 标记（定时任务/手工触发）。
func (s *Service) RefreshTAT() (int, error) {
	var rows []model.ForecastOrder
	if err := s.DB.Where("tenant_id = ? AND full_received_at IS NOT NULL AND closed_at IS NULL", s.Tenant).
		Find(&rows).Error; err != nil {
		return 0, err
	}
	n := 0
	for i := range rows {
		fo := rows[i]
		d := TatDays(*fo.FullReceivedAt, s.now())
		warn := d >= TATWarningDays
		overdue := d > TATLimitDays
		if err := s.DB.Model(&model.ForecastOrder{}).
			Where("tenant_id = ? AND forecast_no = ?", s.Tenant, fo.ForecastNo).
			Updates(map[string]any{"tat_warning": warn, "tat_overdue": overdue}).Error; err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// ReturnProgressView 还货进度看板行（RT-06）。
type ReturnProgressView struct {
	ForecastNo  string         `json:"forecast_no"`
	RMANo       string         `json:"rma_no"`
	RMAStatus   string         `json:"rma_status"`
	SNTotal     int            `json:"sn_total"`
	Returned    int            `json:"returned"`
	Pending     int            `json:"pending"`
	MethodDist  map[string]int `json:"method_dist"`
	TatDays     *int           `json:"tat_days"`
	TatWarning  bool           `json:"tat_warning"`
	TatOverdue  bool           `json:"tat_overdue"`
	ElapsedDays int            `json:"elapsed_days"`
}

// GetReturnProgress 单个 RMA 还货进度（RT-06 / P-09）。
func (s *Service) GetReturnProgress(forecastNo string) (*ReturnProgressView, error) {
	fo, err := s.getForecast(s.DB, forecastNo)
	if err != nil {
		return nil, err
	}
	var sns []model.SNItem
	if err := s.DB.Where("tenant_id = ? AND forecast_no = ?", s.Tenant, forecastNo).Find(&sns).Error; err != nil {
		return nil, err
	}
	var rrs []model.ReturnRecord
	if err := s.DB.Where("tenant_id = ? AND forecast_no = ?", s.Tenant, forecastNo).Find(&rrs).Error; err != nil {
		return nil, err
	}
	dist := map[string]int{}
	shipped := 0
	for _, r := range rrs {
		dist[r.ReturnMethod]++
		if r.Status == "SHIPPED" {
			shipped++
		}
	}
	eligible := 0
	for _, it := range sns {
		if it.Status != string(enum.SNClosedShort) {
			eligible++
		}
	}
	view := &ReturnProgressView{
		ForecastNo: fo.ForecastNo, RMANo: fo.RMANo, RMAStatus: fo.RMAStatus,
		SNTotal: len(sns), Returned: shipped, Pending: eligible - shipped,
		MethodDist: dist, TatDays: fo.TatDays,
		TatWarning: fo.TatWarning, TatOverdue: fo.TatOverdue,
	}
	if view.Pending < 0 {
		view.Pending = 0
	}
	if fo.FullReceivedAt != nil {
		end := s.now()
		if fo.ClosedAt != nil {
			end = *fo.ClosedAt
		}
		view.ElapsedDays = TatDays(*fo.FullReceivedAt, end)
	}
	return view, nil
}

// ListReturnBoard 还货看板列表（DZ-07：仅列表 + TAT 标红，不做图表）。
func (s *Service) ListReturnBoard(status, keyword string, page, size int) (PageResult, error) {
	q := s.DB.Model(&model.ForecastOrder{}).Where("tenant_id = ?", s.Tenant)
	if status != "" {
		q = q.Where("rma_status = ?", status)
	}
	if keyword != "" {
		kw := "%" + keyword + "%"
		q = q.Where("rma_no LIKE ? OR forecast_no LIKE ?", kw, kw)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return PageResult{}, err
	}
	var rows []model.ForecastOrder
	q2, p, sz := paged(q, page, size)
	if err := q2.Order("created_at DESC").Find(&rows).Error; err != nil {
		return PageResult{}, err
	}
	out := make([]ReturnProgressView, 0, len(rows))
	for _, fo := range rows {
		v, err := s.GetReturnProgress(fo.ForecastNo)
		if err != nil {
			return PageResult{}, err
		}
		out = append(out, *v)
	}
	return wrapPage(out, total, p, sz), nil
}

// ErrNotClosed 保留
var ErrNotClosed = errs.New(409, "NOT_CLOSED", "RMA 尚未关闭")
