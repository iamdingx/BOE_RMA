// Package errs 统一领域错误：错误码 + HTTP 状态码 + 中文提示。
package errs

import "fmt"

// AppError 业务错误（可直接映射为 HTTP 响应）。
type AppError struct {
	HTTP int    `json:"-"`
	Code string `json:"code"`
	Msg  string `json:"message"`
}

func (e *AppError) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Msg) }

func New(http int, code, msg string) *AppError { return &AppError{HTTP: http, Code: code, Msg: msg} }

// 常用错误构造
func BadRequest(code, msg string) *AppError   { return New(400, code, msg) }
func Unauthorized(msg string) *AppError       { return New(401, "UNAUTHORIZED", msg) }
func Forbidden(msg string) *AppError          { return New(403, "FORBIDDEN", msg) }
func NotFound(what string) *AppError          { return New(404, "NOT_FOUND", what+"不存在") }
func Conflict(code, msg string) *AppError     { return New(409, code, msg) }
func PayloadTooLarge(msg string) *AppError    { return New(413, "PAYLOAD_TOO_LARGE", msg) }
func Internal(msg string) *AppError           { return New(500, "INTERNAL", msg) }

// 预定义
var (
	ErrIllegalTransition = func(from, to string) *AppError {
		return Conflict("ILLEGAL_TRANSITION", fmt.Sprintf("非法状态跃迁：%s → %s", from, to))
	}
	ErrStateNotAllowed = func(action, cur string) *AppError {
		return Conflict("STATE_NOT_ALLOWED", fmt.Sprintf("当前状态 %s 不允许执行「%s」", cur, action))
	}
	ErrInvalidEnum = func(field, val string) *AppError {
		return BadRequest("INVALID_ENUM", fmt.Sprintf("字段 %s 取值非法：%s", field, val))
	}
	ErrQtyMismatch = func(expect, got int) *AppError {
		return BadRequest("QTY_MISMATCH", fmt.Sprintf("SN 清单条数(%d)与预报数量(%d)不一致", got, expect))
	}
	ErrPhotoRequired = func(n int) *AppError {
		return BadRequest("PHOTO_REQUIRED", fmt.Sprintf("佐证照片不足，至少需要 %d 张", n))
	}
	ErrBoxSeqExhausted = func(day string) *AppError {
		return Conflict("BOX_SEQ_EXHAUSTED", fmt.Sprintf("%s 当日箱号流水已达上限 999", day))
	}
	ErrBufferInsufficient = func(pn string) *AppError {
		return Conflict("BUFFER_INSUFFICIENT", fmt.Sprintf("料号 %s 无可用缓冲备件", pn))
	}
	ErrBufferConcurrent = func(sn string) *AppError {
		return Conflict("BUFFER_CONCURRENT", fmt.Sprintf("备件 %s 已被其他单预占，请重试", sn))
	}
)
