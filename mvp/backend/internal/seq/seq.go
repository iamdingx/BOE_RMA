// Package seq 单号生成：箱号（BOE/OB 两套独立序列）+ 通用业务单号。
// 全部在事务内自增，保证并发唯一；箱号单日上限 999。
package seq

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/boe/rma/internal/errs"
	"github.com/boe/rma/internal/model"
)

const (
	PrefixReceive = "BOE" // 收货箱前缀
	PrefixShip    = "OB"  // 发货箱前缀（与收货箱区分，禁止重号）
	BoxMaxPerDay  = 999
)

// NextBoxNo 生成收货箱号（BOE+YYMMDD+3位）或发货箱号（OB+YYMMDD+3位）。
func NextBoxNo(tx *gorm.DB, tenantID, seqType string, now time.Time) (string, error) {
	var prefix string
	switch seqType {
	case model.SeqReceive:
		prefix = PrefixReceive
	case model.SeqShip:
		prefix = PrefixShip
	default:
		return "", errs.BadRequest("INVALID_SEQ_TYPE", "未知箱号序列类型："+seqType)
	}
	day := now.Format("060102")
	n, err := incrBox(tx, tenantID, seqType, day)
	if err != nil {
		return "", err
	}
	if n > BoxMaxPerDay {
		return "", errs.ErrBoxSeqExhausted(day)
	}
	return fmt.Sprintf("%s%s%03d", prefix, day, n), nil
}

// NextBizNo 生成通用业务单号：prefix + YYMMDD + 固定位数流水。
func NextBizNo(tx *gorm.DB, tenantID, seqType, prefix string, width int, now time.Time) (string, error) {
	day := now.Format("060102")
	n, err := incrBiz(tx, tenantID, seqType, day)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%s%0*d", prefix, day, width, n), nil
}

// incrBox 原子自增箱号序列（行级幂等：先插后加）。
func incrBox(tx *gorm.DB, tenantID, seqType, day string) (int, error) {
	dialect := tx.Dialector.Name()
	var sql string
	if dialect == "mysql" {
		sql = `INSERT INTO box_sequence (tenant_id,seq_type,date_key,current_no) VALUES (?,?,?,1)
		       ON DUPLICATE KEY UPDATE current_no = current_no + 1`
	} else {
		sql = `INSERT INTO box_sequence (tenant_id,seq_type,date_key,current_no) VALUES (?,?,?,1)
		       ON CONFLICT(tenant_id,seq_type,date_key) DO UPDATE SET current_no = current_no + 1`
	}
	if err := tx.Exec(sql, tenantID, seqType, day).Error; err != nil {
		return 0, err
	}
	var row model.BoxSequence
	if err := tx.Where("tenant_id = ? AND seq_type = ? AND date_key = ?", tenantID, seqType, day).
		First(&row).Error; err != nil {
		return 0, err
	}
	return row.CurrentNo, nil
}

// incrBiz 原子自增通用单号序列。
func incrBiz(tx *gorm.DB, tenantID, seqType, day string) (int, error) {
	dialect := tx.Dialector.Name()
	var sql string
	if dialect == "mysql" {
		sql = `INSERT INTO biz_sequence (tenant_id,seq_type,date_key,current_no) VALUES (?,?,?,1)
		       ON DUPLICATE KEY UPDATE current_no = current_no + 1`
	} else {
		sql = `INSERT INTO biz_sequence (tenant_id,seq_type,date_key,current_no) VALUES (?,?,?,1)
		       ON CONFLICT(tenant_id,seq_type,date_key) DO UPDATE SET current_no = current_no + 1`
	}
	if err := tx.Exec(sql, tenantID, seqType, day).Error; err != nil {
		return 0, err
	}
	var row model.BizSequence
	if err := tx.Where("tenant_id = ? AND seq_type = ? AND date_key = ?", tenantID, seqType, day).
		First(&row).Error; err != nil {
		return 0, err
	}
	return row.CurrentNo, nil
}
