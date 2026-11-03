package seq_test

import (
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/boe/rma/internal/model"
	"github.com/boe/rma/internal/seq"
	"github.com/boe/rma/internal/store"
)

func newDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:seqdb%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := store.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func nextBox(t *testing.T, db *gorm.DB, seqType string, at time.Time) (string, error) {
	t.Helper()
	var no string
	var err error
	txErr := db.Transaction(func(tx *gorm.DB) error {
		no, err = seq.NextBoxNo(tx, "BOE_US", seqType, at)
		return err
	})
	if txErr != nil {
		return "", txErr
	}
	return no, nil
}

// TestBoxNoFormatAndIsolation 收货箱 BOE / 发货箱 OB 两套独立序列、不重号。
func TestBoxNoFormatAndIsolation(t *testing.T) {
	db := newDB(t)
	day := time.Date(2026, 11, 3, 10, 0, 0, 0, time.Local)

	no1, err := nextBox(t, db, model.SeqReceive, day)
	if err != nil {
		t.Fatalf("receive: %v", err)
	}
	if no1 != "BOE261103001" {
		t.Fatalf("期望 BOE261103001，实际 %s", no1)
	}
	no2, err := nextBox(t, db, model.SeqReceive, day)
	if err != nil {
		t.Fatalf("receive2: %v", err)
	}
	if no2 != "BOE261103002" {
		t.Fatalf("期望 BOE261103002，实际 %s", no2)
	}
	// 发货箱独立序列，从 001 开始
	ship1, err := nextBox(t, db, model.SeqShip, day)
	if err != nil {
		t.Fatalf("ship: %v", err)
	}
	if ship1 != "OB261103001" {
		t.Fatalf("期望 OB261103001，实际 %s", ship1)
	}
	if ship1 == no1 {
		t.Fatal("收货箱与发货箱禁止重号")
	}
}

// TestBoxNoCrossDayReset 跨日重置为 001。
func TestBoxNoCrossDayReset(t *testing.T) {
	db := newDB(t)
	d1 := time.Date(2026, 11, 3, 23, 59, 0, 0, time.Local)
	d2 := time.Date(2026, 11, 4, 0, 1, 0, 0, time.Local)
	if _, err := nextBox(t, db, model.SeqReceive, d1); err != nil {
		t.Fatalf("d1: %v", err)
	}
	no, err := nextBox(t, db, model.SeqReceive, d2)
	if err != nil {
		t.Fatalf("d2: %v", err)
	}
	if no != "BOE261104001" {
		t.Fatalf("跨日应重置为 BOE261104001，实际 %s", no)
	}
}

// TestBoxNoDailyLimit 单日上限 999。
func TestBoxNoDailyLimit(t *testing.T) {
	db := newDB(t)
	day := time.Date(2026, 11, 3, 10, 0, 0, 0, time.Local)
	// 预置到 999
	if err := db.Create(&model.BoxSequence{
		TenantID: "BOE_US", SeqType: model.SeqReceive, DateKey: "261103", CurrentNo: 999,
	}).Error; err != nil {
		t.Fatalf("seed seq: %v", err)
	}
	if _, err := nextBox(t, db, model.SeqReceive, day); err == nil {
		t.Fatal("第 1000 个箱号应报错（单日上限 999）")
	}
	// 次日仍可用
	next := time.Date(2026, 11, 4, 10, 0, 0, 0, time.Local)
	no, err := nextBox(t, db, model.SeqReceive, next)
	if err != nil {
		t.Fatalf("次日: %v", err)
	}
	if no != "BOE261104001" {
		t.Fatalf("次日应重置，实际 %s", no)
	}
}

// TestBizNoPrefix 通用单号前缀与位数。
func TestBizNoPrefix(t *testing.T) {
	db := newDB(t)
	day := time.Date(2026, 11, 3, 10, 0, 0, 0, time.Local)
	var no string
	if err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		no, err = seq.NextBizNo(tx, "BOE_US", "FORECAST", "F", 4, day)
		return err
	}); err != nil {
		t.Fatalf("biz no: %v", err)
	}
	if no != "F2611030001" {
		t.Fatalf("期望 F2611030001，实际 %s", no)
	}
}
