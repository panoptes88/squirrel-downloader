package handler

import (
	"fmt"
	"net/http"
	"strings"

	"squirrel-downloader/internal/model"

	"github.com/gin-gonic/gin"
)

// 首次初始化：应用安装后第一次访问时由用户设置管理员密码。
//
// 之所以改成这样，是因为原来的做法由安装器把用户输入的 ADMIN_PASSWORD 明文写进
// docker-compose.yaml，合规审核把这一条判为「应用本地明文存储密码」。
// 现在 compose 里连占位符都没有，密码只在内存里出现一次、随后以 bcrypt 入库。

// MinPasswordLen 是设置管理员密码时的最小长度。
const MinPasswordLen = 8

// MaxPasswordLen 与 project.yaml 里原来的参数约束保持一致。
const MaxPasswordLen = 64

type setupRequest struct {
	Password string `json:"password" binding:"required"`
}

// GetSetupStatus 告诉前端是否还需要初始化管理员。
//
// 未初始化时前端显示「设置管理员密码」页面而不是登录页。
func GetSetupStatus(c *gin.Context) {
	var count int64
	if err := model.GetDB().Model(&model.User{}).Count(&count).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取用户数量失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"needs_setup": count == 0})
}

// Setup 在系统中还没有任何管理员时创建第一个管理员，并直接签发会话。
//
// 只要有管理员就不允许再调用，避免被用来覆盖已有账号的密码。
func Setup(c *gin.Context) {
	var count int64
	if err := model.GetDB().Model(&model.User{}).Count(&count).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取用户数量失败"})
		return
	}
	if count > 0 {
		c.JSON(http.StatusForbidden, gin.H{"error": "管理员已存在，无法重复初始化"})
		return
	}

	var req setupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	if strings.TrimSpace(req.Password) != req.Password {
		c.JSON(http.StatusBadRequest, gin.H{"error": "密码首尾不能有空白字符"})
		return
	}
	if len([]rune(req.Password)) < MinPasswordLen {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("密码至少 %d 位", MinPasswordLen)})
		return
	}
	if len([]rune(req.Password)) > MaxPasswordLen {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("密码最多 %d 位", MaxPasswordLen)})
		return
	}

	hashed, err := model.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "密码加密失败"})
		return
	}

	user := model.User{Username: "admin", Password: hashed}
	if err := model.GetDB().Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建管理员失败"})
		return
	}

	// 创建成功后直接登录，省去用户再输一次密码
	issueSession(c, "admin", req.Password)
}
