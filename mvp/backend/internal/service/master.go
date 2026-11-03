package service

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/boe/rma/internal/auth"
	"github.com/boe/rma/internal/enum"
	"github.com/boe/rma/internal/errs"
	"github.com/boe/rma/internal/model"
	"github.com/boe/rma/internal/seq"
)

// ---------- 认证 ----------

// LoginReq 登录入参。
type LoginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResp 登录结果。
type LoginResp struct {
	Token    string `json:"token"`
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Display  string `json:"display_name"`
}

// Login 登录（客户角色一期不开放，C-09）。
func (s *Service) Login(req LoginReq, signer *auth.Signer) (*LoginResp, error) {
	var u model.User
	if err := s.DB.Where("tenant_id = ? AND username = ?", s.Tenant, req.Username).First(&u).Error; err != nil {
		return nil, errs.Unauthorized("用户名或密码错误")
	}
	if !u.Enabled {
		return nil, errs.Forbidden("账号已停用")
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)) != nil {
		return nil, errs.Unauthorized("用户名或密码错误")
	}
	// 一期客户不开放访问（C-09 / UX-16）
	if enum.Role(u.Role) == enum.RoleCustomer {
		return nil, errs.Forbidden("客户账号一期不开放系统访问（二期门户开放）")
	}
	token, err := signer.Sign(u.TenantID, u.UserID, u.Username, enum.Role(u.Role), s.now())
	if err != nil {
		return nil, err
	}
	_ = s.DB.Create(&model.AuditLog{
		TenantID: s.Tenant, BizType: "sys_user", BizNo: u.UserID, Action: string(enum.AuditLogin),
		Operator: u.Username, CreatedAt: s.now(),
	}).Error
	return &LoginResp{
		Token: token, UserID: u.UserID, Username: u.Username, Role: u.Role, Display: u.DisplayName,
	}, nil
}

// ---------- 主数据（MD-01/02/03） ----------

func (s *Service) ListModels() ([]model.ProductModel, error) {
	var rows []model.ProductModel
	err := s.DB.Where("tenant_id = ?", s.Tenant).Order("model_code").Find(&rows).Error
	return rows, err
}

func (s *Service) CreateModel(code, name string) (*model.ProductModel, error) {
	code = trimAll(code)
	if code == "" || name == "" {
		return nil, errs.BadRequest("INVALID_PARAM", "机型编码与名称必填")
	}
	m := &model.ProductModel{TenantID: s.Tenant, ModelCode: code, ModelName: name, CreatedAt: s.now()}
	if err := s.DB.Create(m).Error; err != nil {
		return nil, errs.Conflict("MODEL_EXISTS", "机型编码已存在："+code)
	}
	return m, nil
}

func (s *Service) ListPNs(modelCode string) ([]model.PartNumber, error) {
	q := s.DB.Where("tenant_id = ?", s.Tenant)
	if modelCode != "" {
		q = q.Where("model_code = ?", modelCode)
	}
	var rows []model.PartNumber
	err := q.Order("pn_code").Find(&rows).Error
	return rows, err
}

func (s *Service) CreatePN(code, modelCode, desc string) (*model.PartNumber, error) {
	code = trimAll(code)
	modelCode = trimAll(modelCode)
	if code == "" || modelCode == "" {
		return nil, errs.BadRequest("INVALID_PARAM", "料号与机型必填")
	}
	var cnt int64
	s.DB.Model(&model.ProductModel{}).Where("tenant_id = ? AND model_code = ?", s.Tenant, modelCode).Count(&cnt)
	if cnt == 0 {
		return nil, errs.BadRequest("MODEL_NOT_FOUND", "机型 "+modelCode+" 不存在")
	}
	p := &model.PartNumber{TenantID: s.Tenant, PNCode: code, ModelCode: modelCode, Description: desc, CreatedAt: s.now()}
	if err := s.DB.Create(p).Error; err != nil {
		return nil, errs.Conflict("PN_EXISTS", "料号已存在："+code)
	}
	return p, nil
}

// ListUsers 用户列表（MD-02）。
func (s *Service) ListUsers() ([]model.User, error) {
	var rows []model.User
	err := s.DB.Where("tenant_id = ?", s.Tenant).Order("username").Find(&rows).Error
	return rows, err
}

// CreateUser 创建用户（5 角色固定）。
func (s *Service) CreateUser(username, password, display string, role enum.Role) (*model.User, error) {
	if !role.Valid() {
		return nil, errs.ErrInvalidEnum("role", string(role))
	}
	if username == "" || len(password) < 6 {
		return nil, errs.BadRequest("INVALID_PARAM", "用户名必填，密码至少 6 位")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	uid, err := seq.NextBizNo(s.DB, s.Tenant, "USER", "U-", 4, s.now())
	if err != nil {
		return nil, err
	}
	u := &model.User{
		TenantID: s.Tenant, UserID: uid, Username: username, PasswordHash: string(hash),
		DisplayName: display, Role: string(role), Enabled: true, CreatedAt: s.now(),
	}
	if err := s.DB.Create(u).Error; err != nil {
		return nil, errs.Conflict("USER_EXISTS", "用户名已存在")
	}
	return u, nil
}

// ---------- 照片（PH-01~04） ----------

// SavePhotoReq 照片落库入参。
type SavePhotoReq struct {
	BizType     enum.PhotoBizType
	BizRefNo    string
	SN          string
	PNCode      string
	FileName    string
	ContentType string
	Size        int64
	ObjectKey   string
	OriginalURL string
	ThumbURL    string
	UploadedBy  string
}

// MaxPhotoSize 单张上限 10MB（PH-02）。
const MaxPhotoSize int64 = 10 * 1024 * 1024

// SavePhoto 校验并落库照片（≥2 年留存）。
func (s *Service) SavePhoto(req SavePhotoReq) (*model.PhotoEvidence, error) {
	if req.Size > MaxPhotoSize {
		return nil, errs.PayloadTooLarge("单张照片不得超过 10MB")
	}
	if req.Size <= 0 {
		return nil, errs.BadRequest("EMPTY_FILE", "照片内容为空")
	}
	if !req.BizType.Valid() {
		return nil, errs.ErrInvalidEnum("biz_type", string(req.BizType))
	}
	if req.BizRefNo == "" {
		return nil, errs.BadRequest("INVALID_PARAM", "业务关联号必填")
	}
	if !isImage(req.ContentType, req.FileName) {
		return nil, errs.BadRequest("INVALID_FILE_TYPE", "仅支持图片文件（jpg/png/webp/heic）")
	}
	pid, err := seq.NextBizNo(s.DB, s.Tenant, "PHOTO", "PH", 6, s.now())
	if err != nil {
		return nil, err
	}
	p := &model.PhotoEvidence{
		TenantID: s.Tenant, PhotoID: pid, BizType: string(req.BizType), BizRefNo: req.BizRefNo,
		SN: req.SN, PNCode: req.PNCode, ObjectKey: req.ObjectKey,
		OriginalURL: req.OriginalURL, ThumbnailURL: req.ThumbURL,
		FileSize: req.Size, ContentType: req.ContentType,
		UploadedBy: req.UploadedBy, UploadedAt: s.now(),
		RetentionUntil: s.now().AddDate(2, 0, 0),
	}
	if err := s.DB.Create(p).Error; err != nil {
		return nil, err
	}
	_ = s.DB.Create(&model.AuditLog{
		TenantID: s.Tenant, BizType: "photo_evidence", BizNo: pid, Action: string(enum.AuditPhotoUpload),
		Operator: req.UploadedBy, NewValue: req.ObjectKey, CreatedAt: s.now(),
	}).Error
	return p, nil
}

func isImage(contentType, name string) bool {
	if strings.HasPrefix(strings.ToLower(contentType), "image/") {
		return true
	}
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".heic", ".gif", ".bmp":
		return true
	}
	return false
}

// ListPhotos 按业务关联查照片。
func (s *Service) ListPhotos(bizType, bizRefNo string) ([]model.PhotoEvidence, error) {
	q := s.DB.Where("tenant_id = ?", s.Tenant)
	if bizType != "" {
		q = q.Where("biz_type = ?", bizType)
	}
	if bizRefNo != "" {
		q = q.Where("biz_ref_no = ?", bizRefNo)
	}
	var rows []model.PhotoEvidence
	err := q.Order("uploaded_at").Find(&rows).Error
	return rows, err
}

// PurgeExpiredPhotos 保留期提示（不删除，PH-03 只增不删）。
func (s *Service) PurgeExpiredPhotos() (int64, error) {
	var cnt int64
	err := s.DB.Model(&model.PhotoEvidence{}).
		Where("tenant_id = ? AND retention_until < ?", s.Tenant, s.now()).Count(&cnt).Error
	return cnt, err
}

var (
	_ = fmt.Sprint
	_ = gorm.ErrRecordNotFound
	_ = time.Now
)
