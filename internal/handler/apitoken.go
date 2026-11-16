package handler

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"squirrel-downloader/internal/model"

	"github.com/gin-gonic/gin"
)

// API 令牌管理（设置页使用）。
//
// 目的：让 Agent / 脚本不必实现 RSA + AES 加密就能调用业务接口。
// 用户在设置页创建令牌、设置有效期，之后直接放进 Authorization: Bearer。

// MaxAPITokens 是未撤销令牌的数量上限。
const MaxAPITokens = 5

// 允许的有效期档位，前端下拉框与本表一一对应。
var apiTokenTTLs = map[string]time.Duration{
	"1d":    24 * time.Hour,
	"1w":    7 * 24 * time.Hour,
	"1m":    30 * 24 * time.Hour,
	"3m":    90 * 24 * time.Hour,
	"6m":    180 * 24 * time.Hour,
	"1y":    365 * 24 * time.Hour,
	"never": 0, // 永久
}

type createAPITokenRequest struct {
	Name      string `json:"name" binding:"required"`
	ExpiresIn string `json:"expires_in" binding:"required"`
}

// ListAPITokens 返回全部未撤销的令牌。
//
// 只返回元信息（名称、前缀、有效期、最后使用时间），永远不返回明文或哈希。
func ListAPITokens(c *gin.Context) {
	var tokens []model.APIToken
	if err := model.GetDB().Where("revoked_at IS NULL").Order("created_at DESC").Find(&tokens).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取令牌列表失败"})
		return
	}
	c.JSON(http.StatusOK, tokens)
}

// CreateAPIToken 创建令牌。
//
// 响应里的 token 字段是**唯一一次**能看到明文的机会，之后数据库只留哈希。
func CreateAPIToken(c *gin.Context) {
	var req createAPITokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "令牌名称不能为空"})
		return
	}
	if len([]rune(name)) > 32 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "令牌名称最多 32 个字符"})
		return
	}

	// 数量上限：未撤销的令牌最多 10 个。
	// 令牌泄露时需要能快速定位并撤销，数量太多会难以排查；也避免误操作刷满。
	var active int64
	if err := model.GetDB().Model(&model.APIToken{}).Where("revoked_at IS NULL").Count(&active).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取令牌数量失败"})
		return
	}
	if active >= MaxAPITokens {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("最多只能创建 %d 个有效令牌，请先撤销不用的令牌", MaxAPITokens),
		})
		return
	}

	ttl, ok := apiTokenTTLs[req.ExpiresIn]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "有效期取值无效"})
		return
	}

	plain, token, err := model.GenerateAPIToken(name, ttl)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成令牌失败"})
		return
	}
	if err := model.GetDB().Create(token).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存令牌失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token": plain, // 仅此一次
		"item":  token,
	})
}

// RevokeAPIToken 撤销令牌。撤销后立即失效，记录保留以便审计。
func RevokeAPIToken(c *gin.Context) {
	id := c.Param("id")
	var token model.APIToken
	if err := model.GetDB().First(&token, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "令牌不存在"})
		return
	}
	if token.RevokedAt != nil {
		c.JSON(http.StatusOK, gin.H{"message": "令牌已撤销"})
		return
	}

	now := time.Now()
	if err := model.GetDB().Model(&token).Update("revoked_at", now).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "撤销令牌失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "令牌已撤销"})
}
