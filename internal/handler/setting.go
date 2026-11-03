package handler

import (
	"net/http"

	"squirrel-downloader/internal/config"
	"squirrel-downloader/internal/service"

	"github.com/gin-gonic/gin"
)

// GetSettings 返回当前生效的运行期设置
func GetSettings(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"max_concurrent_tasks": service.GetMaxConcurrent(config.Load()),
	})
}

type UpdateSettingsRequest struct {
	MaxConcurrentTasks *int `json:"max_concurrent_tasks" binding:"required"`
}

// UpdateSettings 修改运行期设置，下一次任务轮询即生效
func UpdateSettings(c *gin.Context) {
	var req UpdateSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	if err := service.SetMaxConcurrent(*req.MaxConcurrentTasks); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"max_concurrent_tasks": service.GetMaxConcurrent(config.Load()),
	})
}
