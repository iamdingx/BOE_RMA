package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"github.com/boe/rma/internal/enum"
)

// ---------- 自定义 JSON 列类型 ----------

// StrList 字符串数组，落库为 JSON 文本。
type StrList []string

func (s StrList) Value() (driver.Value, error) {
	if s == nil {
		return nil, nil
	}
	b, err := json.Marshal([]string(s))
	return string(b), err
}

func (s *StrList) Scan(v any) error {
	if v == nil {
		*s = nil
		return nil
	}
	var raw []byte
	switch t := v.(type) {
	case []byte:
		raw = t
	case string:
		raw = []byte(t)
	default:
		return errors.New("StrList: unsupported type")
	}
	if len(raw) == 0 {
		*s = nil
		return nil
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil {
		return err
	}
	*s = out
	return nil
}

// JSONMap 通用 JSON 对象，用于审批留痕/打印日志。
type JSONMap map[string]any

func (m JSONMap) Value() (driver.Value, error) {
	if m == nil {
		return nil, nil
	}
	b, err := json.Marshal(m)
	return string(b), err
}

func (m *JSONMap) Scan(v any) error {
	if v == nil {
		*m = nil
		return nil
	}
	var raw []byte
	switch t := v.(type) {
	case []byte:
		raw = t
	case string:
		raw = []byte(t)
	default:
		return errors.New("JSONMap: unsupported type")
	}
	if len(raw) == 0 {
		*m = nil
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return err
	}
	*m = out
	return nil
}

// ---------- E10 租户 / 主数据 ----------

type Tenant struct {
	TenantID   string    `gorm:"column:tenant_id;size:32;primaryKey" json:"tenant_id"`
	TenantName string    `gorm:"column:tenant_name;size:128;not null" json:"tenant_name"`
	CreatedAt  time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (Tenant) TableName() string { return "tenant" }

type ProductModel struct {
	TenantID  string    `gorm:"column:tenant_id;size:32;primaryKey" json:"tenant_id"`
	ModelCode string    `gorm:"column:model_code;size:64;primaryKey" json:"model_code"`
	ModelName string    `gorm:"column:model_name;size:128;not null" json:"model_name"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (ProductModel) TableName() string { return "product_model" }

type PartNumber struct {
	TenantID    string    `gorm:"column:tenant_id;size:32;primaryKey" json:"tenant_id"`
	PNCode      string    `gorm:"column:pn_code;size:64;primaryKey" json:"pn_code"`
	ModelCode   string    `gorm:"column:model_code;size:64;not null;index" json:"model_code"`
	Description string    `gorm:"column:description;size:255" json:"description"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (PartNumber) TableName() string { return "part_number" }

type User struct {
	TenantID     string    `gorm:"column:tenant_id;size:32;primaryKey" json:"tenant_id"`
	UserID       string    `gorm:"column:user_id;size:32;primaryKey" json:"user_id"`
	Username     string    `gorm:"column:username;size:64;not null;uniqueIndex:uk_user_name,priority:2" json:"username"`
	PasswordHash string    `gorm:"column:password_hash;size:128;not null" json:"-"`
	DisplayName  string    `gorm:"column:display_name;size:64;not null" json:"display_name"`
	Role         string    `gorm:"column:role;size:32;not null" json:"role"`
	Enabled      bool      `gorm:"column:enabled;not null;default:true" json:"enabled"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (User) TableName() string { return "sys_user" }

// ---------- E1 预报单 / RMA ----------

type ForecastOrder struct {
	TenantID        string     `gorm:"column:tenant_id;size:32;primaryKey" json:"tenant_id"`
	ForecastNo      string     `gorm:"column:forecast_no;size:32;primaryKey" json:"forecast_no"`
	RMANo           string     `gorm:"column:rma_no;size:64;not null;uniqueIndex:uk_rma,priority:2" json:"rma_no"`
	PNCode          string     `gorm:"column:pn_code;size:64;not null" json:"pn_code"`
	ModelCode       string     `gorm:"column:model_code;size:64" json:"model_code"`
	ForecastQty     int        `gorm:"column:forecast_qty;not null;default:0" json:"forecast_qty"`
	Customer        string     `gorm:"column:customer;size:128;not null" json:"customer"`
	Status          string     `gorm:"column:status;size:16;not null;default:ENTERED" json:"status"`
	RMAStatus       string     `gorm:"column:rma_status;size:24;not null;default:OPEN;index" json:"rma_status"`
	CreatedBy       string     `gorm:"column:created_by;size:64;not null" json:"created_by"`
	CreatedAt       time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	FullReceivedAt  *time.Time `gorm:"column:full_received_at" json:"full_received_at"`
	ClosedAt        *time.Time `gorm:"column:closed_at" json:"closed_at"`
	TatDays         *int       `gorm:"column:tat_days" json:"tat_days"`
	TatWarning      bool       `gorm:"column:tat_warning;not null;default:false" json:"tat_warning"`
	TatOverdue      bool       `gorm:"column:tat_overdue;not null;default:false" json:"tat_overdue"`
	LabelQtyTotal   *int       `gorm:"column:label_qty_total" json:"label_qty_total"`
}

func (ForecastOrder) TableName() string { return "forecast_order" }

type ForecastSNItem struct {
	TenantID   string `gorm:"column:tenant_id;size:32;primaryKey" json:"tenant_id"`
	ForecastNo string `gorm:"column:forecast_no;size:32;primaryKey" json:"forecast_no"`
	SN         string `gorm:"column:sn;size:128;primaryKey;index" json:"sn"`
	RMANo      string `gorm:"column:rma_no;size:64;not null" json:"rma_no"`
	PNCode     string `gorm:"column:pn_code;size:64;not null" json:"pn_code"`
}

func (ForecastSNItem) TableName() string { return "forecast_sn_item" }

// ---------- E3 允收批次 ----------

type ReceivingBatch struct {
	TenantID      string    `gorm:"column:tenant_id;size:32;primaryKey" json:"tenant_id"`
	BatchNo       string    `gorm:"column:batch_no;size:32;primaryKey" json:"batch_no"`
	ForecastNo    string    `gorm:"column:forecast_no;size:32;not null;index" json:"forecast_no"`
	PodPhotoIDs   StrList   `gorm:"column:pod_photo_ids;type:text" json:"pod_photo_ids"`
	LabelQtyTotal int       `gorm:"column:label_qty_total;not null;default:0" json:"label_qty_total"`
	QtyDiff       int       `gorm:"column:qty_diff;not null;default:0" json:"qty_diff"`
	Status        string    `gorm:"column:status;size:16;not null;default:RECEIVED" json:"status"`
	ReceivedBy    string    `gorm:"column:received_by;size:64;not null" json:"received_by"`
	ReceivedAt    time.Time `gorm:"column:received_at;autoCreateTime" json:"received_at"`
	IsLateReceive bool      `gorm:"column:is_late_receive;not null;default:false" json:"is_late_receive"`
}

func (ReceivingBatch) TableName() string { return "receiving_batch" }

// ---------- E3-B 收货箱 ----------

type BoxInfo struct {
	TenantID      string    `gorm:"column:tenant_id;size:32;primaryKey" json:"tenant_id"`
	BoxNo         string    `gorm:"column:box_no;size:32;primaryKey" json:"box_no"`
	BatchNo       string    `gorm:"column:batch_no;size:32;not null;index" json:"batch_no"`
	ForecastNo    string    `gorm:"column:forecast_no;size:32;not null" json:"forecast_no"`
	SNList        StrList   `gorm:"column:sn_list;type:text" json:"sn_list"`
	Status        string    `gorm:"column:status;size:16;not null;default:OPEN" json:"status"`
	LabelPrintLog JSONMap   `gorm:"column:label_print_log;type:text" json:"label_print_log"`
	SealedBy      string    `gorm:"column:sealed_by;size:64" json:"sealed_by"`
	SealedAt      *time.Time `gorm:"column:sealed_at" json:"sealed_at"`
	CreatedBy     string    `gorm:"column:created_by;size:64;not null" json:"created_by"`
	CreatedAt     time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (BoxInfo) TableName() string { return "box_info" }

type BoxSNItem struct {
	TenantID string `gorm:"column:tenant_id;size:32;primaryKey" json:"tenant_id"`
	BoxNo    string `gorm:"column:box_no;size:32;primaryKey" json:"box_no"`
	SN       string `gorm:"column:sn;size:128;primaryKey;index" json:"sn"`
}

func (BoxSNItem) TableName() string { return "box_sn_item" }

// ---------- E4 SN 单品 ----------

type SNItem struct {
	TenantID            string     `gorm:"column:tenant_id;size:32;primaryKey" json:"tenant_id"`
	SN                  string     `gorm:"column:sn;size:128;primaryKey" json:"sn"`
	Status              string     `gorm:"column:status;size:24;not null;index" json:"status"`
	RMANo               string     `gorm:"column:rma_no;size:64;index" json:"rma_no"`
	PNCode              string     `gorm:"column:pn_code;size:64" json:"pn_code"`
	ForecastNo          string     `gorm:"column:forecast_no;size:32;index" json:"forecast_no"`
	BatchNo             string     `gorm:"column:batch_no;size:32" json:"batch_no"`
	BoxNo               string     `gorm:"column:box_no;size:32;index" json:"box_no"`
	JudgmentResult      string     `gorm:"column:judgment_result;size:16" json:"judgment_result"`
	ReturnMethod        string     `gorm:"column:return_method;size:24" json:"return_method"`
	OutboundNo          string     `gorm:"column:outbound_no;size:32" json:"outbound_no"`
	ReturnCount         int        `gorm:"column:return_count;not null;default:0" json:"return_count"`
	SecondaryReturnFlag bool       `gorm:"column:secondary_return_flag;not null;default:false" json:"secondary_return_flag"`
	CIDFlag             bool       `gorm:"column:cid_flag;not null;default:false" json:"cid_flag"`
	Owner               string     `gorm:"column:owner;size:64" json:"owner"`
	Locked              bool       `gorm:"column:locked;not null;default:false" json:"locked"`
	ReceivedAt          *time.Time `gorm:"column:received_at" json:"received_at"`
	InspectedAt         *time.Time `gorm:"column:inspected_at" json:"inspected_at"`
	JudgedAt            *time.Time `gorm:"column:judged_at" json:"judged_at"`
	OutboundAt          *time.Time `gorm:"column:outbound_at" json:"outbound_at"`
	CompletedAt         *time.Time `gorm:"column:completed_at" json:"completed_at"`
	UpdatedAt           time.Time  `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (SNItem) TableName() string { return "sn_item" }

// ---------- E4-B SN 历史轮次 ----------

type SNItemRound struct {
	TenantID       string     `gorm:"column:tenant_id;size:32;primaryKey" json:"tenant_id"`
	RoundID        string     `gorm:"column:round_id;size:40;primaryKey" json:"round_id"`
	SN             string     `gorm:"column:sn;size:128;not null;index" json:"sn"`
	RoundNo        int        `gorm:"column:round_no;not null" json:"round_no"`
	RMANo          string     `gorm:"column:rma_no;size:64" json:"rma_no"`
	PNCode         string     `gorm:"column:pn_code;size:64" json:"pn_code"`
	ForecastNo     string     `gorm:"column:forecast_no;size:32" json:"forecast_no"`
	FromStatus     string     `gorm:"column:from_status;size:24;not null" json:"from_status"`
	ToStatus       string     `gorm:"column:to_status;size:24;not null" json:"to_status"`
	StartedAt      *time.Time `gorm:"column:started_at" json:"started_at"`
	EndedAt        *time.Time `gorm:"column:ended_at" json:"ended_at"`
	ReceivedAt     *time.Time `gorm:"column:received_at" json:"received_at"`
	JudgedAt       *time.Time `gorm:"column:judged_at" json:"judged_at"`
	OutboundAt     *time.Time `gorm:"column:outbound_at" json:"outbound_at"`
	JudgmentResult string     `gorm:"column:judgment_result;size:16" json:"judgment_result"`
	OutboundType   string     `gorm:"column:outbound_type;size:24" json:"outbound_type"`
}

func (SNItemRound) TableName() string { return "sn_item_round" }

// ---------- E5 差异记录 ----------

type DiscrepancyRecord struct {
	TenantID        string    `gorm:"column:tenant_id;size:32;primaryKey" json:"tenant_id"`
	RecordNo        string    `gorm:"column:record_no;size:40;primaryKey" json:"record_no"`
	Category        string    `gorm:"column:category;size:24;not null" json:"category"`
	Source          string    `gorm:"column:source;size:16;not null" json:"source"`
	DiscrepancyType string    `gorm:"column:discrepancy_type;size:24;not null" json:"discrepancy_type"`
	ForecastNo      string    `gorm:"column:forecast_no;size:32;index" json:"forecast_no"`
	BatchNo         string    `gorm:"column:batch_no;size:32" json:"batch_no"`
	BoxNo           string    `gorm:"column:box_no;size:32" json:"box_no"`
	SN              string    `gorm:"column:sn;size:128;index" json:"sn"`
	PNCode          string    `gorm:"column:pn_code;size:64" json:"pn_code"`
	RMANo           string    `gorm:"column:rma_no;size:64" json:"rma_no"`
	MissingSNList   StrList   `gorm:"column:missing_sn_list;type:text" json:"missing_sn_list"`
	QtyDiff         int       `gorm:"column:qty_diff;not null;default:0" json:"qty_diff"`
	PhotoIDs        StrList   `gorm:"column:photo_ids;type:text" json:"photo_ids"`
	ApprovalStatus  string    `gorm:"column:approval_status;size:16;not null;default:PENDING;index" json:"approval_status"`
	Approver        string    `gorm:"column:approver;size:64" json:"approver"`
	ApprovedAt      *time.Time `gorm:"column:approved_at" json:"approved_at"`
	ApprovalNote    string    `gorm:"column:approval_note;type:text" json:"approval_note"`
	RejectReason    string    `gorm:"column:reject_reason;type:text" json:"reject_reason"`
	DiscoveredBy    string    `gorm:"column:discovered_by;size:64;not null" json:"discovered_by"`
	DiscoveredAt    time.Time `gorm:"column:discovered_at;autoCreateTime" json:"discovered_at"`
	Locked          bool      `gorm:"column:locked;not null" json:"locked"`
}

func (DiscrepancyRecord) TableName() string { return "discrepancy_record" }

// ---------- E6 质检判定 ----------

type InspectionDisposition struct {
	TenantID             string    `gorm:"column:tenant_id;size:32;primaryKey" json:"tenant_id"`
	RecordID             string    `gorm:"column:record_id;size:40;primaryKey" json:"record_id"`
	SN                   string    `gorm:"column:sn;size:128;not null;index" json:"sn"`
	JudgmentResult       string    `gorm:"column:judgment_result;size:16;not null" json:"judgment_result"`
	CIDFlag              bool      `gorm:"column:cid_flag;not null;default:false" json:"cid_flag"`
	DamagePhotoIDs       StrList   `gorm:"column:damage_photo_ids;type:text" json:"damage_photo_ids"`
	CustomerConfirmation string    `gorm:"column:customer_confirmation;type:text" json:"customer_confirmation"`
	Inspector            string    `gorm:"column:inspector;size:64;not null" json:"inspector"`
	InspectionAt         time.Time `gorm:"column:inspection_at;autoCreateTime" json:"inspection_at"`
}

func (InspectionDisposition) TableName() string { return "inspection_disposition" }

// ---------- E7 出库单 ----------

type OutboundOrder struct {
	TenantID         string     `gorm:"column:tenant_id;size:32;primaryKey" json:"tenant_id"`
	OutboundNo       string     `gorm:"column:outbound_no;size:32;primaryKey" json:"outbound_no"`
	OutboundType     string     `gorm:"column:outbound_type;size:24;not null;index" json:"outbound_type"`
	Status           string     `gorm:"column:status;size:20;not null;default:CREATED;index" json:"status"`
	RMANo            string     `gorm:"column:rma_no;size:64" json:"rma_no"`
	LogisticsNo      string     `gorm:"column:logistics_no;size:64" json:"logistics_no"`
	Carrier          string     `gorm:"column:carrier;size:64" json:"carrier"`
	ShippedAt        *time.Time `gorm:"column:shipped_at" json:"shipped_at"`
	ScrapApproval    JSONMap    `gorm:"column:scrap_approval;type:text" json:"scrap_approval"`
	TransferNo       string     `gorm:"column:transfer_no;size:64" json:"transfer_no"`
	CountConfirmedBy string     `gorm:"column:count_confirmed_by;size:64" json:"count_confirmed_by"`
	CountConfirmedAt *time.Time `gorm:"column:count_confirmed_at" json:"count_confirmed_at"`
	CIDFlag          bool       `gorm:"column:cid_flag;not null;default:false" json:"cid_flag"`
	Operator         string     `gorm:"column:operator;size:64;not null" json:"operator"`
	OutboundAt       *time.Time `gorm:"column:outbound_at" json:"outbound_at"`
	CreatedAt        time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (OutboundOrder) TableName() string { return "outbound_order" }

type OutboundSNItem struct {
	TenantID   string `gorm:"column:tenant_id;size:32;primaryKey" json:"tenant_id"`
	OutboundNo string `gorm:"column:outbound_no;size:32;primaryKey" json:"outbound_no"`
	SN         string `gorm:"column:sn;size:128;primaryKey;index" json:"sn"`
}

func (OutboundSNItem) TableName() string { return "outbound_sn_item" }

// ---------- E7-B 发货箱 ----------

type ShipmentBox struct {
	TenantID      string     `gorm:"column:tenant_id;size:32;primaryKey" json:"tenant_id"`
	ShipmentBoxNo string     `gorm:"column:shipment_box_no;size:32;primaryKey" json:"shipment_box_no"`
	RMANos        StrList    `gorm:"column:rma_nos;type:text" json:"rma_nos"`
	OutboundNo    string     `gorm:"column:outbound_no;size:32;not null;index" json:"outbound_no"`
	SNList        StrList    `gorm:"column:sn_list;type:text" json:"sn_list"`
	Weight        *float64   `gorm:"column:weight;type:decimal(10,3)" json:"weight"`
	Volume        *float64   `gorm:"column:volume;type:decimal(10,4)" json:"volume"`
	Carrier       string     `gorm:"column:carrier;size:64" json:"carrier"`
	TrackingNo    string     `gorm:"column:tracking_no;size:64" json:"tracking_no"`
	LabelPrintLog JSONMap    `gorm:"column:label_print_log;type:text" json:"label_print_log"`
	PackedBy      string     `gorm:"column:packed_by;size:64;not null" json:"packed_by"`
	PackedAt      time.Time  `gorm:"column:packed_at;autoCreateTime" json:"packed_at"`
	Sealed        bool       `gorm:"column:sealed;not null;default:false" json:"sealed"`
	ShippedAt     *time.Time `gorm:"column:shipped_at" json:"shipped_at"`
}

func (ShipmentBox) TableName() string { return "shipment_box" }

type ShipmentBoxSN struct {
	TenantID      string `gorm:"column:tenant_id;size:32;primaryKey" json:"tenant_id"`
	ShipmentBoxNo string `gorm:"column:shipment_box_no;size:32;primaryKey" json:"shipment_box_no"`
	SN            string `gorm:"column:sn;size:128;primaryKey;index" json:"sn"`
}

func (ShipmentBoxSN) TableName() string { return "shipment_box_sn" }

// ---------- E8 照片 ----------

type PhotoEvidence struct {
	TenantID       string    `gorm:"column:tenant_id;size:32;primaryKey" json:"tenant_id"`
	PhotoID        string    `gorm:"column:photo_id;size:40;primaryKey" json:"photo_id"`
	BizType        string    `gorm:"column:biz_type;size:32;not null;index" json:"biz_type"`
	BizRefNo       string    `gorm:"column:biz_ref_no;size:64;not null;index" json:"biz_ref_no"`
	SN             string    `gorm:"column:sn;size:128" json:"sn"`
	PNCode         string    `gorm:"column:pn_code;size:64" json:"pn_code"`
	ObjectKey      string    `gorm:"column:object_key;size:255;not null" json:"object_key"`
	OriginalURL    string    `gorm:"column:original_url;size:512;not null" json:"original_url"`
	ThumbnailURL   string    `gorm:"column:thumbnail_url;size:512" json:"thumbnail_url"`
	FileSize       int64     `gorm:"column:file_size;not null" json:"file_size"`
	ContentType    string    `gorm:"column:content_type;size:64" json:"content_type"`
	UploadedBy     string    `gorm:"column:uploaded_by;size:64;not null" json:"uploaded_by"`
	UploadedAt     time.Time `gorm:"column:uploaded_at;autoCreateTime" json:"uploaded_at"`
	RetentionUntil time.Time `gorm:"column:retention_until;type:date;not null" json:"retention_until"`
}

func (PhotoEvidence) TableName() string { return "photo_evidence" }

// ---------- E9 审计日志（只增不删） ----------

type AuditLog struct {
	TenantID   string    `gorm:"column:tenant_id;size:32;not null;index" json:"tenant_id"`
	LogID      int64     `gorm:"column:log_id;primaryKey;autoIncrement" json:"log_id"`
	BizType    string    `gorm:"column:biz_type;size:32;not null;index" json:"biz_type"`
	BizNo      string    `gorm:"column:biz_no;size:64;not null;index" json:"biz_no"`
	Action     string    `gorm:"column:action;size:40;not null;index" json:"action"`
	Field      string    `gorm:"column:field;size:64" json:"field"`
	OldValue   string    `gorm:"column:old_value;type:text" json:"old_value"`
	NewValue   string    `gorm:"column:new_value;type:text" json:"new_value"`
	Reason     string    `gorm:"column:reason;type:text" json:"reason"`
	Operator   string    `gorm:"column:operator;size:64;not null" json:"operator"`
	ApprovedBy string    `gorm:"column:approved_by;size:64" json:"approved_by"`
	CreatedAt  time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (AuditLog) TableName() string { return "audit_log" }

// ---------- 箱号序列 ----------

type BoxSequence struct {
	TenantID  string `gorm:"column:tenant_id;size:32;primaryKey" json:"tenant_id"`
	SeqType   string `gorm:"column:seq_type;size:16;primaryKey" json:"seq_type"`
	DateKey   string `gorm:"column:date_key;size:6;primaryKey" json:"date_key"`
	CurrentNo int    `gorm:"column:current_no;not null;default:0" json:"current_no"`
}

func (BoxSequence) TableName() string { return "box_sequence" }

const (
	SeqReceive = "RECEIVE" // BOE 前缀
	SeqShip    = "SHIP"    // OB 前缀
)

// BizSequence 通用业务单号序列（预报/批次/出库/还货/差异/审批等）。
type BizSequence struct {
	TenantID  string `gorm:"column:tenant_id;size:32;primaryKey" json:"tenant_id"`
	SeqType   string `gorm:"column:seq_type;size:24;primaryKey" json:"seq_type"`
	DateKey   string `gorm:"column:date_key;size:6;primaryKey" json:"date_key"`
	CurrentNo int    `gorm:"column:current_no;not null;default:0" json:"current_no"`
}

func (BizSequence) TableName() string { return "biz_sequence" }

// ---------- E11 缓冲备件 ----------

type BufferStock struct {
	TenantID       string     `gorm:"column:tenant_id;size:32;primaryKey" json:"tenant_id"`
	SN             string     `gorm:"column:sn;size:128;primaryKey" json:"sn"`
	PNCode         string     `gorm:"column:pn_code;size:64;not null;index" json:"pn_code"`
	Source         string     `gorm:"column:source;size:20;not null" json:"source"`
	StockStatus    string     `gorm:"column:stock_status;size:16;not null;default:AVAILABLE;index" json:"stock_status"`
	InboundOrderNo string     `gorm:"column:inbound_order_no;size:32" json:"inbound_order_no"`
	InboundAt      time.Time  `gorm:"column:inbound_at;autoCreateTime" json:"inbound_at"`
	InboundBy      string     `gorm:"column:inbound_by;size:64;not null" json:"inbound_by"`
	ReturnRecordNo string     `gorm:"column:return_record_no;size:40" json:"return_record_no"`
	ReservedBy     string     `gorm:"column:reserved_by;size:40" json:"reserved_by"`
	ReservedAt     *time.Time `gorm:"column:reserved_at" json:"reserved_at"`
}

func (BufferStock) TableName() string { return "buffer_stock" }

type BufferInboundOrder struct {
	TenantID    string    `gorm:"column:tenant_id;size:32;primaryKey" json:"tenant_id"`
	OrderNo     string    `gorm:"column:order_no;size:32;primaryKey" json:"order_no"`
	PNCode      string    `gorm:"column:pn_code;size:64;not null" json:"pn_code"`
	Source      string    `gorm:"column:source;size:20;not null" json:"source"`
	QtyExpected int       `gorm:"column:qty_expected;not null" json:"qty_expected"`
	Status      string    `gorm:"column:status;size:16;not null;default:CREATED" json:"status"`
	CreatedBy   string    `gorm:"column:created_by;size:64;not null" json:"created_by"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (BufferInboundOrder) TableName() string { return "buffer_inbound_order" }

type BufferSubstituteApproval struct {
	TenantID     string     `gorm:"column:tenant_id;size:32;primaryKey" json:"tenant_id"`
	ApprovalNo   string     `gorm:"column:approval_no;size:40;primaryKey" json:"approval_no"`
	OriginalSN   string     `gorm:"column:original_sn;size:128;not null" json:"original_sn"`
	OriginalPN   string     `gorm:"column:original_pn;size:64;not null" json:"original_pn"`
	SubstituteSN string     `gorm:"column:substitute_sn;size:128;not null" json:"substitute_sn"`
	SubstitutePN string     `gorm:"column:substitute_pn;size:64;not null" json:"substitute_pn"`
	Status       string     `gorm:"column:status;size:16;not null;default:PENDING;index" json:"status"`
	Approver     string     `gorm:"column:approver;size:64" json:"approver"`
	ApprovedAt   *time.Time `gorm:"column:approved_at" json:"approved_at"`
	Note         string     `gorm:"column:note;type:text" json:"note"`
	RequestedBy  string     `gorm:"column:requested_by;size:64;not null" json:"requested_by"`
	CreatedAt    time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (BufferSubstituteApproval) TableName() string { return "buffer_substitute_approval" }

// ---------- E12 还货记录 ----------

type ReturnRecord struct {
	TenantID            string     `gorm:"column:tenant_id;size:32;primaryKey" json:"tenant_id"`
	ReturnNo            string     `gorm:"column:return_no;size:40;primaryKey" json:"return_no"`
	RMANo               string     `gorm:"column:rma_no;size:64;not null;index" json:"rma_no"`
	ForecastNo          string     `gorm:"column:forecast_no;size:32;not null;uniqueIndex:uk_rr_orig_round,priority:2" json:"forecast_no"`
	OriginalSN          string     `gorm:"column:original_sn;size:128;not null;uniqueIndex:uk_rr_orig_round,priority:3;index" json:"original_sn"`
	ReturnMethod        string     `gorm:"column:return_method;size:24;not null" json:"return_method"`
	ReturnedSN          string     `gorm:"column:returned_sn;size:128;not null" json:"returned_sn"`
	OutboundNo          string     `gorm:"column:outbound_no;size:32" json:"outbound_no"`
	ShipmentBoxNo       string     `gorm:"column:shipment_box_no;size:32" json:"shipment_box_no"`
	OriginalDisposition string     `gorm:"column:original_disposition;size:24;not null" json:"original_disposition"`
	RetainedDisposition string     `gorm:"column:retained_disposition;size:24" json:"retained_disposition"`
	Status              string     `gorm:"column:status;size:16;not null;default:PENDING" json:"status"`
	ReturnedAt          *time.Time `gorm:"column:returned_at" json:"returned_at"`
	Operator            string     `gorm:"column:operator;size:64" json:"operator"`
	CreatedAt           time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (ReturnRecord) TableName() string { return "return_record" }

// AllModels 供 AutoMigrate 使用（顺序满足外键依赖）。
func AllModels() []any {
	return []any{
		&Tenant{}, &ProductModel{}, &PartNumber{}, &User{},
		&ForecastOrder{}, &ForecastSNItem{},
		&ReceivingBatch{}, &BoxInfo{}, &BoxSNItem{},
		&SNItem{}, &SNItemRound{},
		&DiscrepancyRecord{}, &InspectionDisposition{},
		&OutboundOrder{}, &OutboundSNItem{},
		&ShipmentBox{}, &ShipmentBoxSN{},
		&PhotoEvidence{}, &AuditLog{}, &BoxSequence{}, &BizSequence{},
		&BufferStock{}, &BufferInboundOrder{}, &BufferSubstituteApproval{},
		&ReturnRecord{},
	}
}

var _ = enum.SNForecasted // 保持 enum 依赖显式
