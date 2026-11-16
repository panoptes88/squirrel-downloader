package middleware

import (
	"net/http"
	"strings"

	"squirrel-downloader/internal/model"

	"github.com/gin-gonic/gin"
)

// AuthRequired 校验会话。
//
// 支持两种携带方式，按顺序取第一个非空的：
//  1. Cookie: auth_token=<token>          —— 浏览器 / Web 界面
//  2. Authorization: Bearer <token>       —— Agent / 脚本 / 自动化工具
//
// 第二种是为了让调用方不必维护 Cookie Jar：登录接口同时返回 access_token，
// 之后直接放进 Authorization 头即可。
func AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := bearerToken(c)

		if token == "" {
			if t, err := c.Cookie("auth_token"); err == nil {
				token = t
			}
		}

		if token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录"})
			c.Abort()
			return
		}

		// API 令牌：给 Agent / 脚本用的长期凭证，不绑定具体会话。
		// 前缀 sq_ 便于和会话 token（UUID）区分。
		if model.LooksLikeAPIToken(token) {
			if _, err := model.ValidateAPIToken(model.GetDB(), token); err != nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "API 令牌无效、已撤销或已过期"})
				c.Abort()
				return
			}
			c.Set("auth_via", "api_token")
			c.Next()
			return
		}

		// 验证会话 token
		if _, err := model.GetUserByToken(model.GetDB(), token); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "登录已过期，请重新登录"})
			c.Abort()
			return
		}

		c.Set("auth_via", "session")
		c.Next()
	}
}

// bearerToken 从 Authorization 头里取出 Bearer token，没有则返回空串。
// scheme 大小写不敏感（RFC 7235）。
func bearerToken(c *gin.Context) string {
	header := strings.TrimSpace(c.GetHeader("Authorization"))
	if header == "" {
		return ""
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

// SessionOnly 只允许登录会话通过，拒绝 API 令牌。
//
// 账号管理类操作（改密、注销、创建/撤销令牌）必须走这条链路：
// 长期令牌一旦泄露，不应该能借它重置密码或再签发新令牌。
func SessionOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetString("auth_via") == "api_token" {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "该操作需要使用登录会话，API 令牌无权执行",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}
