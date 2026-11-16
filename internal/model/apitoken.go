package model

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"gorm.io/gorm"
)

// APIToken 是给自动化调用方（Agent、脚本、工作流工具）使用的长期令牌。
//
// 与登录会话的区别：
//   - 会话（Session）绑定浏览器 Cookie，24 小时过期，由登录流程创建
//   - 令牌（APIToken）由用户在设置页主动创建，可设 1 天到永久，可随时撤销
//
// 安全设计：
//   - 数据库只存 SHA-256 哈希，明文仅在创建时返回一次
//   - 令牌有固定前缀 sq_，便于在日志和代码里识别，也便于中间件快速分流
//   - 令牌只能调用业务接口，不能用于改密、注销、创建新令牌等账号管理操作
type APIToken struct {
	ID     uint   `gorm:"primarykey" json:"id"`
	Name   string `gorm:"size:64" json:"name"`
	Hash   string `gorm:"uniqueIndex;size:64" json:"-"`
	Prefix string `gorm:"size:16" json:"prefix"`

	// ExpiresAt 为 nil 表示永久有效
	ExpiresAt *time.Time `json:"expires_at,omitempty"`

	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`

	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

// TokenPrefix 是所有 API 令牌的固定前缀。
const TokenPrefix = "sq_"

// ErrTokenInvalid 表示令牌不存在、已撤销或已过期。
var ErrTokenInvalid = errors.New("令牌无效或已过期")

// GenerateAPIToken 生成一个新令牌，返回明文与可直接入库的记录。
//
// 明文格式：sq_<32 字节随机数的 base64url>，长度约 46 字符。
func GenerateAPIToken(name string, ttl time.Duration) (string, *APIToken, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, err
	}
	plain := TokenPrefix + base64.RawURLEncoding.EncodeToString(raw)

	token := &APIToken{
		Name:   name,
		Hash:   HashAPIToken(plain),
		Prefix: plain[:min(len(plain), 10)],
	}
	if ttl > 0 {
		t := time.Now().Add(ttl)
		token.ExpiresAt = &t
	}
	return plain, token, nil
}

// HashAPIToken 计算令牌的 SHA-256 哈希。
//
// 这里不用 bcrypt：令牌本身是 32 字节的密码学随机数，没有低熵问题，
// 用 SHA-256 即可，且能走唯一索引做 O(1) 查找。
func HashAPIToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

// ValidateAPIToken 校验令牌并返回对应记录。
// 校验通过时会顺手更新最后使用时间（失败不影响鉴权）。
func ValidateAPIToken(db *gorm.DB, plain string) (*APIToken, error) {
	if plain == "" {
		return nil, ErrTokenInvalid
	}

	var token APIToken
	if err := db.Where("hash = ?", HashAPIToken(plain)).First(&token).Error; err != nil {
		return nil, ErrTokenInvalid
	}
	if token.RevokedAt != nil {
		return nil, ErrTokenInvalid
	}
	if token.ExpiresAt != nil && time.Now().After(*token.ExpiresAt) {
		return nil, ErrTokenInvalid
	}

	now := time.Now()
	token.LastUsedAt = &now
	// 只更新这一列，避免并发写入冲突；失败也不阻断请求
	db.Model(&token).UpdateColumn("last_used_at", now)

	return &token, nil
}

// LooksLikeAPIToken 判断一个 Bearer 值是不是 API 令牌。
func LooksLikeAPIToken(v string) bool {
	return len(v) > len(TokenPrefix) && v[:len(TokenPrefix)] == TokenPrefix
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
