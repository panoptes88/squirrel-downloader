package handler

import (
	"net/http"

	"squirrel-downloader/internal/config"
	"squirrel-downloader/internal/model"
	"squirrel-downloader/internal/service"

	"github.com/gin-gonic/gin"
)

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// issueSession 校验凭据并写入会话，成功时已自行写响应。
// 明文登录与密文登录共用，避免两处逻辑漂移。
func issueSession(c *gin.Context, username, password string) {
	user, err := service.GetUserByUsername(username)
	if err != nil || !model.CheckPassword(password, user.Password) {
		// 统一错误信息，防止用户名枚举
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
		return
	}

	// 创建会话，生成随机 token
	session, err := model.CreateSession(model.GetDB(), user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建会话失败"})
		return
	}

	// 设置 cookie，有效期 24 小时
	cfg := config.Load()
	c.SetCookie("auth_token", session.Token, 86400, "/", "", !cfg.AllowInsecure, true)

	// 同时把 token 放进响应体：Agent / 脚本可以放进 Authorization: Bearer 头，
	// 不必维护 Cookie Jar。浏览器端仍然走 Cookie。
	c.JSON(http.StatusOK, gin.H{
		"id":           user.ID,
		"username":     user.Username,
		"access_token": session.Token,
		"expires_in":   86400,
	})
}

func Logout(c *gin.Context) {
	token, err := c.Cookie("auth_token")
	if err == nil && token != "" {
		model.DeleteSession(model.GetDB(), token)
	}
	c.SetCookie("auth_token", "", -1, "/", "", !config.Load().AllowInsecure, true)
	c.JSON(http.StatusOK, gin.H{"message": "登出成功"})
}

func GetUser(c *gin.Context) {
	token, err := c.Cookie("auth_token")
	if err != nil || token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	user, err := model.GetUserByToken(model.GetDB(), token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "登录已过期"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"username": user.Username,
	})
}

type ChangePasswordRequest struct {
	// OldPassword 可选：明文接口为了兼容旧前端允许不传，
	// 密文接口则强制要求，见 ChangePasswordEncrypted。
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password" binding:"required"`
}

// applyPasswordChange 校验旧密码、检查新密码强度并落库。
//
// 校验旧密码是为了防止会话 token 泄露后被直接改密顶掉原主人；
// 明文与密文两条路径共用这里。
func applyPasswordChange(c *gin.Context, user *model.User, oldPassword, newPassword string) {
	if oldPassword != "" && !model.CheckPassword(oldPassword, user.Password) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "原密码不正确"})
		return
	}

	if err := service.ValidatePassword(newPassword); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 先吊销全部会话再改密：即使改密失败，也只是需要重新登录，
	// 不会出现「密码已改但旧会话仍然有效」的窗口
	if err := model.DeleteSessionsByUser(model.GetDB(), user.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "修改密码失败"})
		return
	}

	if err := service.ChangePassword(user.Username, newPassword); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "修改密码失败"})
		return
	}

	c.SetCookie("auth_token", "", -1, "/", "", !config.Load().AllowInsecure, true)

	c.JSON(http.StatusOK, gin.H{"message": "密码修改成功，请重新登录"})
}
