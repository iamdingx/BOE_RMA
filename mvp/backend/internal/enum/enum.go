// Package enum 集中固化 PRD V1.22 6.2 全部枚举。
// 禁止在业务代码中出现裸字符串，一律引用本包常量。
package enum

// ---------------- ① SN 状态 ----------------
type SNStatus string

const (
	SNForecasted  SNStatus = "FORECASTED"   // 已预报
	SNReceived    SNStatus = "RECEIVED"     // 已接收
	SNDiscrepancy SNStatus = "DISCREPANCY"  // 差异处理中
	SNInspecting  SNStatus = "INSPECTING"   // 质检中
	SNJudged      SNStatus = "JUDGED"       // 已判定
	SNOutbound    SNStatus = "OUTBOUND"     // 已出库（动作完成，终态未定）
	SNCompleted   SNStatus = "COMPLETED"    // 完成：正常发客户/调拨
	SNClosedShort SNStatus = "CLOSED_SHORT" // 少收关单（终态）
	SNRetained    SNStatus = "RETAINED"     // 留库待定（中间态，非终态）
	SNDisposed    SNStatus = "DISPOSED"     // 已处置（处置类终态）
)

var AllSNStatus = []SNStatus{
	SNForecasted, SNReceived, SNDiscrepancy, SNInspecting, SNJudged,
	SNOutbound, SNCompleted, SNClosedShort, SNRetained, SNDisposed,
}

func (s SNStatus) Valid() bool {
	for _, v := range AllSNStatus {
		if v == s {
			return true
		}
	}
	return false
}

// ---------------- ② 判定结果 ----------------
type Judgment string

const (
	JudgeRepair     Judgment = "REPAIR"      // 维修
	JudgeReplace    Judgment = "REPLACE"     // 换新
	JudgeScrap      Judgment = "SCRAP"       // 报废
	JudgeTransferRC Judgment = "TRANSFER_RC" // 转维修中心
	JudgeCID        Judgment = "CID_RETURN"  // CID 原片返还（客户责）
)

var AllJudgments = []Judgment{JudgeRepair, JudgeReplace, JudgeScrap, JudgeTransferRC, JudgeCID}

func (j Judgment) Valid() bool {
	for _, v := range AllJudgments {
		if v == j {
			return true
		}
	}
	return false
}

// ---------------- ③ 出库类型 ----------------
type OutboundType string

const (
	OutShipToCustomer   OutboundType = "SHIP_TO_CUSTOMER"
	OutShipToRC         OutboundType = "SHIP_TO_RC"
	OutScrapDisposal    OutboundType = "SCRAP_DISPOSAL"
	OutTransferWarehouse OutboundType = "TRANSFER_WAREHOUSE"
)

var AllOutboundTypes = []OutboundType{OutShipToCustomer, OutShipToRC, OutScrapDisposal, OutTransferWarehouse}

func (o OutboundType) Valid() bool {
	for _, v := range AllOutboundTypes {
		if v == o {
			return true
		}
	}
	return false
}

// ---------------- ④ 差异类型 ----------------
type DiscrepancyType string

const (
	DiscSNNotInForecast DiscrepancyType = "SN_NOT_IN_FORECAST"
	DiscPNMismatch      DiscrepancyType = "PN_MISMATCH"
	DiscRMAMismatch     DiscrepancyType = "RMA_MISMATCH"
	DiscQtyOver         DiscrepancyType = "QTY_OVER"
	DiscQtyShort        DiscrepancyType = "QTY_SHORT"
)

func (d DiscrepancyType) Valid() bool {
	switch d {
	case DiscSNNotInForecast, DiscPNMismatch, DiscRMAMismatch, DiscQtyOver, DiscQtyShort:
		return true
	}
	return false
}

// ---------------- ⑤ 异常类别 ----------------
type AnomalyCategory string

const (
	AnomalyForecast AnomalyCategory = "FORECAST_ANOMALY"
	AnomalyArrival  AnomalyCategory = "ARRIVAL_ANOMALY"
)

// ---------------- ⑥ 预报单状态 ----------------
type ForecastStatus string

const (
	ForecastEntered ForecastStatus = "ENTERED"
	ForecastRevised ForecastStatus = "REVISED"
)

// ---------------- ⑦ 差异审核状态（REJECTED：DC-04 必要扩展 R-001） ----------------
type ApprovalStatus string

const (
	ApprovalPending  ApprovalStatus = "PENDING"
	ApprovalApproved ApprovalStatus = "APPROVED"
	ApprovalRejected ApprovalStatus = "REJECTED"
)

// ---------------- ⑧ 出库单状态 ----------------
type OutboundStatus string

const (
	OutCreated         OutboundStatus = "CREATED"
	OutPendingApproval OutboundStatus = "PENDING_APPROVAL"
	OutRejected        OutboundStatus = "REJECTED"
	OutShipped         OutboundStatus = "SHIPPED"
)

// ---------------- ⑨ 照片业务类型（POD/BOX_LABEL/SHIPMENT：PH-03 扩展 R-002） ----------------
type PhotoBizType string

const (
	PhotoDiscrepancy PhotoBizType = "DISCREPANCY_EVIDENCE"
	PhotoCIDDamage   PhotoBizType = "CID_DAMAGE_EVIDENCE"
	PhotoPOD         PhotoBizType = "POD"
	PhotoBoxLabel    PhotoBizType = "BOX_LABEL"
	PhotoShipment    PhotoBizType = "SHIPMENT"
)

func (p PhotoBizType) Valid() bool {
	switch p {
	case PhotoDiscrepancy, PhotoCIDDamage, PhotoPOD, PhotoBoxLabel, PhotoShipment:
		return true
	}
	return false
}

// ---------------- ⑩ RMA 状态 ----------------
type RMAStatus string

const (
	RMAOpen          RMAStatus = "OPEN"
	RMAReceiving     RMAStatus = "RECEIVING"
	RMAProcessing    RMAStatus = "PROCESSING"
	RMAPendingReturn RMAStatus = "PENDING_RETURN"
	RMAReturned      RMAStatus = "RETURNED"
	RMAClosed        RMAStatus = "CLOSED"
)

var AllRMAStatus = []RMAStatus{RMAOpen, RMAReceiving, RMAProcessing, RMAPendingReturn, RMAReturned, RMAClosed}

// ---------------- ⑪ 还货方式 ----------------
type ReturnMethod string

const (
	ReturnOriginalRepair ReturnMethod = "ORIGINAL_REPAIR"
	ReturnCID            ReturnMethod = "CID_RETURN"
	ReturnBuffer         ReturnMethod = "BUFFER_RETURN"
)

func (r ReturnMethod) Valid() bool {
	switch r {
	case ReturnOriginalRepair, ReturnCID, ReturnBuffer:
		return true
	}
	return false
}

// ---------------- ⑫ 缓冲备件状态 ----------------
type BufferStatus string

const (
	BufferAvailable BufferStatus = "AVAILABLE"
	BufferReserved  BufferStatus = "RESERVED"
	BufferOut       BufferStatus = "OUT"
	// BufferScrapped 备件报废（L-03 单独流程，不进退货 SN 状态机；见风险 R-021）
	BufferScrapped BufferStatus = "SCRAPPED"
)

// ---------------- ⑬ 原片处置 ----------------
type OriginalDisposition string

const (
	OrigShipped       OriginalDisposition = "ORIGINAL_SHIPPED"
	OrigRetained      OriginalDisposition = "RETAINED"
	OrigScrapped      OriginalDisposition = "SCRAPPED"
	OrigTransferredRC OriginalDisposition = "TRANSFERRED_RC"
)

// ---------------- ⑭ RETAINED 后续处置 ----------------
type RetainedDisposition string

const (
	RetScrapped       RetainedDisposition = "SCRAPPED"
	RetReturnVendor   RetainedDisposition = "RETURN_VENDOR"
	RetRepairToBuffer RetainedDisposition = "REPAIR_TO_BUFFER"
	RetLongRetain     RetainedDisposition = "LONG_RETAIN"
)

func (r RetainedDisposition) Valid() bool {
	switch r {
	case RetScrapped, RetReturnVendor, RetRepairToBuffer, RetLongRetain:
		return true
	}
	return false
}

// ---------------- 角色 ----------------
type Role string

const (
	RoleWarehouse Role = "warehouse_operator"
	RoleProject   Role = "project_staff"
	RoleInspector Role = "inspector"
	RoleCustomer  Role = "customer"
	RoleAdmin     Role = "admin"
)

func (r Role) Valid() bool {
	switch r {
	case RoleWarehouse, RoleProject, RoleInspector, RoleCustomer, RoleAdmin:
		return true
	}
	return false
}

// ---------------- 审计动作（M-14） ----------------
type AuditAction string

const (
	AuditControlledCorrection AuditAction = "CONTROLLED_CORRECTION"
	AuditReturnMethodChange   AuditAction = "RETURN_METHOD_CHANGE"
	AuditRetainedDispose      AuditAction = "RETAINED_DISPOSE"
	AuditBufferSubstitute     AuditAction = "BUFFER_SUBSTITUTE_APPROVAL"
	AuditScrapApproval        AuditAction = "SCRAP_APPROVAL"
	AuditRMAClose             AuditAction = "RMA_CLOSE"
	AuditDiscApprove          AuditAction = "DISCREPANCY_APPROVE"
	AuditDiscReject           AuditAction = "DISCREPANCY_REJECT"
	AuditInspectionJudge      AuditAction = "INSPECTION_JUDGE"
	AuditOutboundShip         AuditAction = "OUTBOUND_SHIP"
	AuditBoxSeal              AuditAction = "BOX_SEAL"
	AuditPhotoUpload          AuditAction = "PHOTO_UPLOAD"
	AuditLogin                AuditAction = "LOGIN"
	AuditStatusTransition     AuditAction = "STATUS_TRANSITION"
)

// 差异来源（DC-11）
const (
	SourceReceiving = "RECEIVING"
	SourceKitting   = "KITTING"
)

// DeriveReturnMethod 三段式：判定 → 自动带出还货方式（RT-01 / D8）。
// REPAIR→原片修复；CID_RETURN→CID；REPLACE/SCRAP/TRANSFER_RC→BUFFER 还货。
func DeriveReturnMethod(j Judgment) ReturnMethod {
	switch j {
	case JudgeRepair:
		return ReturnOriginalRepair
	case JudgeCID:
		return ReturnCID
	default: // REPLACE / SCRAP / TRANSFER_RC
		return ReturnBuffer
	}
}

// DeriveOriginalDisposition 原片最终去向（⑬）。
func DeriveOriginalDisposition(j Judgment) OriginalDisposition {
	switch j {
	case JudgeRepair, JudgeCID:
		return OrigShipped
	case JudgeReplace:
		return OrigRetained
	case JudgeScrap:
		return OrigScrapped
	case JudgeTransferRC:
		return OrigTransferredRC
	}
	return OrigRetained
}

// IsTerminal 判定是否为终态。
func (s SNStatus) IsTerminal() bool {
	return s == SNCompleted || s == SNClosedShort || s == SNDisposed
}
