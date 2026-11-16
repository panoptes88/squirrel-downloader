package handler

import (
	"net/http"
	"strings"

	"squirrel-downloader/internal/model"
	"squirrel-downloader/internal/service"

	"github.com/gin-gonic/gin"
)

// 登录与改密的密文传输接口。
//
// 合规要求「应使用 HTTPS 或密文传输应用个人信息」。HTTPS 依赖用户自己配证书，
// 所以这里再提供一条应用层加密通道，明文 HTTP 下抓包也拿不到密码。
//
// 流程：
//
//	1. GET  /api/auth/encryption-key
//	   → { key_id, algorithm, public_key, challenge, expires_at }
//	2. 客户端随机生成 AES-256 密钥，用 RSA-OAEP-SHA256 加密它；
//	   再用 AES-256-GCM 加密 {"username","password","challenge"}。
//	3. POST /api/auth/login-encrypted
//	   → 服务端解密、消费 challenge、按普通登录处理。
//
// 原来的明文接口保留，插件与脚本不受影响。

// GetAuthEncryptionKey 下发 RSA 公钥与一次性 challenge。
//
// 无需鉴权：登录前就要调用。
func GetAuthEncryptionKey(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, service.GetAuthCrypto().IssueKey())
}

// LoginEncrypted 用密文登录。
func LoginEncrypted(c *gin.Context) {
	var payload service.EncryptedPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	plain, err := service.GetAuthCrypto().Open(&payload)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if plain.Username == "" || plain.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "用户名或密码不能为空"})
		return
	}

	issueSession(c, plain.Username, plain.Password)
}

// ChangePasswordEncrypted 用密文修改密码。
//
// 需要先登录：会话校验走 Authorization 头或 Cookie，与其它受保护接口一致。
func ChangePasswordEncrypted(c *gin.Context) {
	token := bearerOrCookieToken(c)
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	user, err := model.GetUserByToken(model.GetDB(), token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "登录已过期"})
		return
	}

	var payload service.EncryptedPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	plain, err := service.GetAuthCrypto().Open(&payload)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if plain.OldPassword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "原密码不能为空"})
		return
	}
	if plain.NewPassword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "新密码不能为空"})
		return
	}

	applyPasswordChange(c, user, plain.OldPassword, plain.NewPassword)
}

// bearerOrCookieToken 取会话 token：优先 Authorization: Bearer，其次 Cookie。
// 与 middleware.AuthRequired 的顺序保持一致。
func bearerOrCookieToken(c *gin.Context) string {
	if h := c.GetHeader("Authorization"); h != "" {
		if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
			return strings.TrimSpace(h[7:])
		}
	}
	if t, err := c.Cookie("auth_token"); err == nil {
		return t
	}
	return ""
}
