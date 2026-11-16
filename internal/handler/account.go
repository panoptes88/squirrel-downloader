package handler

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"squirrel-downloader/internal/config"
	"squirrel-downloader/internal/model"
	"squirrel-downloader/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// 注销账号（合规第 5 条）。
//
// 本应用是单用户自托管架构：账号由环境变量 ADMIN_PASSWORD 在首次启动时创建，
// 没有注册入口。如果照搬多用户应用的「删完就退出」，用户会被永久锁在门外，
// 只能重装或重启容器。
//
// 因此这里的语义是「销毁账号与全部数据，然后以用户当场指定的新密码重建」：
//
//   - 旧账号、旧密码哈希、全部会话、任务、方案、设置、日志、缩略图全部删除
//   - 用户当场设置新密码，旧密码立即失效
//   - 删除与重建在同一个事务里，任一步失败整体回滚，不会出现「账号没了又没重建」
//
// 可选勾选是否连下载目录里的视频一起删；默认不勾，避免误删用户资产。

// purgeAccountMaxFiles 限制单次注销时删除的视频文件数量，
// 防止下载目录异常庞大时把请求拖死。超出部分会记日志提示用户手动清理。
const purgeAccountMaxFiles = 5000

// purgeableExtensions 与文件管理页展示的范围保持一致。
var purgeableExtensions = map[string]bool{
	".mp4": true, ".mkv": true, ".ts": true, ".m4v": true, ".webm": true, ".mov": true,
	".srt": true, ".ass": true, ".ssa": true, ".vtt": true, ".sub": true,
}

// PurgeAccount 注销账号并重建。
//
// 请求体是密文（与登录、改密同一套 RSA-OAEP + AES-GCM），
// 明文为 {"old_password","new_password","delete_files"}。
func PurgeAccount(c *gin.Context) {
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入当前密码以确认注销"})
		return
	}
	if !model.CheckPassword(plain.OldPassword, user.Password) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "当前密码错误，无法注销账号"})
		return
	}
	if plain.NewPassword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请设置新密码"})
		return
	}
	if err := service.ValidatePassword(plain.NewPassword); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	hashed, err := model.HashPassword(plain.NewPassword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "账号注销失败"})
		return
	}

	// 删除与重建放在同一事务：任一步失败整体回滚，账号保持可用。
	err = model.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&model.Session{}).Error; err != nil {
			return err
		}
		if err := tx.Where("1 = 1").Delete(&model.Task{}).Error; err != nil {
			return err
		}
		if err := tx.Where("1 = 1").Delete(&model.DownloadProfile{}).Error; err != nil {
			return err
		}
		if err := tx.Where("1 = 1").Delete(&model.Setting{}).Error; err != nil {
			return err
		}
		if err := tx.Where("1 = 1").Delete(&model.User{}).Error; err != nil {
			return err
		}
		return tx.Create(&model.User{Username: user.Username, Password: hashed}).Error
	})
	if err != nil {
		log.Printf("注销账号失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "账号注销失败，请稍后重试"})
		return
	}

	// 重建后清掉 cookie，前端会跳到登录页
	if cfg := config.Load(); cfg != nil {
		c.SetCookie("auth_token", "", -1, "/", "", !cfg.AllowInsecure, true)
	}

	// 文件清理放在事务之后：删不掉只是残留，不影响账号已经重置。
	// 失败只记日志，不让接口报错，否则用户会以为注销没成功。
	cfg := config.Load()
	removed := 0
	if cfg != nil {
		removed = purgeFiles(cfg, plain.DeleteFiles)
	}

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"message":       "账号与全部数据已清除，请用新密码登录",
		"removed_files": removed,
	})
}

// purgeFiles 清理日志、缩略图，以及（按需）下载目录里的视频与字幕。
// 返回删除的文件数量，仅用于反馈展示。
func purgeFiles(cfg *config.Config, deleteDownloads bool) int {
	removed := 0

	// 1) 任务日志
	logDir := filepath.Join(cfg.DownloadDir, "Logs")
	if entries, err := os.ReadDir(logDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if os.Remove(filepath.Join(logDir, e.Name())) == nil {
				removed++
			}
		}
	}

	// 2) 缩略图缓存（与 Thumbnail 处理函数使用同一路径）
	thumbDir := filepath.Join(filepath.Dir(cfg.DBPath), "thumbnails")
	if entries, err := os.ReadDir(thumbDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if os.Remove(filepath.Join(thumbDir, e.Name())) == nil {
				removed++
			}
		}
	}

	if !deleteDownloads {
		return removed
	}

	// 3) 下载目录顶层的视频与字幕。只处理顶层，不递归，
	//    避免误伤用户自己放进去的其他目录。
	entries, err := os.ReadDir(cfg.DownloadDir)
	if err != nil {
		log.Printf("读取下载目录失败: %v", err)
		return removed
	}
	skipped := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !purgeableExtensions[strings.ToLower(filepath.Ext(e.Name()))] {
			continue
		}
		if removed >= purgeAccountMaxFiles {
			skipped++
			continue
		}
		if os.Remove(filepath.Join(cfg.DownloadDir, e.Name())) == nil {
			removed++
		}
	}
	if skipped > 0 {
		log.Printf("注销：下载目录文件超过上限，%d 个未删除，请手动清理", skipped)
	}
	return removed
}
