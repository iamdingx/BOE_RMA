// Package auth JWT 签发与解析。
package auth

import (
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/boe/rma/internal/enum"
	"github.com/boe/rma/internal/errs"
)

// Claims JWT 载荷（含租户与角色，供 RBAC 使用）。
type Claims struct {
	TenantID string `json:"tenant_id"`
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// Signer 签发器。
type Signer struct {
	Secret []byte
	TTL    time.Duration
}

func NewSigner(secret string, ttl time.Duration) *Signer {
	return &Signer{Secret: []byte(secret), TTL: ttl}
}

// Sign 生成 token。
func (s *Signer) Sign(tenantID, userID, username string, role enum.Role, now time.Time) (string, error) {
	claims := Claims{
		TenantID: tenantID, UserID: userID, Username: username, Role: string(role),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "boe-rma",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.TTL)),
			Subject:   userID,
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.Secret)
}

// Parse 解析并校验 token。
func (s *Signer) Parse(token string) (*Claims, error) {
	parsed, err := jwt.ParseWithClaims(token, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errs.Unauthorized("签名算法不符")
		}
		return s.Secret, nil
	})
	if err != nil || !parsed.Valid {
		return nil, errs.Unauthorized("token 无效或已过期")
	}
	claims, ok := parsed.Claims.(*Claims)
	if !ok {
		return nil, errs.Unauthorized("token 载荷异常")
	}
	return claims, nil
}
