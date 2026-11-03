package service

import (
	"fmt"
	"strconv"

	"squirrel-downloader/internal/config"
	"squirrel-downloader/internal/model"
)

// SettingMaxConcurrent 是「同时下载任务数」在 settings 表里的键
const SettingMaxConcurrent = "max_concurrent_tasks"

// MaxConcurrentLimit 是页面上允许设置的上限
const MaxConcurrentLimit = 10

// GetMaxConcurrent 返回当前生效的并发数：
// 数据库里有页面设置的值就用它，否则回退到环境变量（MAX_CONCURRENT_TASKS）。
func GetMaxConcurrent(cfg *config.Config) int {
	if v, ok := model.GetSetting(model.GetDB(), SettingMaxConcurrent); ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return cfg.MaxConcurrent
}

// SetMaxConcurrent 持久化页面上的设置，下一次轮询立即生效
func SetMaxConcurrent(n int) error {
	if n < 1 || n > MaxConcurrentLimit {
		return fmt.Errorf("同时下载任务数需在 1-%d 之间", MaxConcurrentLimit)
	}
	return model.SetSetting(model.GetDB(), SettingMaxConcurrent, strconv.Itoa(n))
}
