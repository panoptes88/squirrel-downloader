package handler

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"squirrel-downloader/internal/config"
	"squirrel-downloader/internal/service"

	"github.com/gin-gonic/gin"
)

// 视频 MIME 类型映射（包级别变量，避免重复创建）
var videoMimeTypes = map[string]string{
	".mp4":  "video/mp4",
	".webm": "video/webm",
	".mkv":  "video/x-matroska",
	".avi":  "video/x-msvideo",
	".mov":  "video/quicktime",
	".flv":  "video/x-flv",
	".wmv":  "video/x-ms-wmv",
	".m4v":  "video/x-m4v",
	".3gp":  "video/3gpp",
	".mpg":  "video/mpeg",
	".mpeg": "video/mpeg",
}

func ListFiles(c *gin.Context) {
	cfg := config.Load()
	files, err := service.ListFiles(cfg.DownloadDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, files)
}

func DownloadFile(c *gin.Context) {
	filename := c.Query("name")
	if filename == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "文件名不能为空"})
		return
	}

	// 防止路径遍历
	filename = filepath.Base(filename)

	cfg := config.Load()
	filePath := filepath.Join(cfg.DownloadDir, filename)

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "文件不存在"})
		return
	}

	c.Header("Content-Description", "File Transfer")
	c.Header("Accept-Ranges", "bytes")
	c.Header("Content-Type", getVideoMimeType(filename))
	// 使用 mime.FormatMediaType 生成 RFC 5987 兼容的头：
	// 同时给出 ASCII 回退名与 filename*=UTF-8''…，
	// 此前直接用 url.PathEscape 拼 filename=，中文文件名在部分浏览器会乱码
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{
		"filename": filename,
	}))
	c.File(filePath)
}

// Thumbnail 返回视频的首帧缩略图（JPEG）。
//
// 浏览器端抽帧需要先加载整个视频，开销太大，所以放到服务端用 ffmpeg 抽一帧，
// 并按「文件名 + 大小 + 修改时间」缓存到应用数据目录，避免重复抽帧。
//
// 注意用的是系统 ffmpeg（/usr/bin/ffmpeg，带完整编码器与 swscale），
// 而不是 /app/bin 下那个只够转封装的精简版。
func Thumbnail(c *gin.Context) {
	filename := filepath.Base(c.Query("name"))
	if filename == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "文件名不能为空"})
		return
	}

	cfg := config.Load()
	src := filepath.Join(cfg.DownloadDir, filename)
	info, err := os.Stat(src)
	if err != nil || info.IsDir() {
		c.Status(http.StatusNotFound)
		return
	}

	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}

	// 缓存键带上大小与修改时间：文件被重新下载后会生成新的缩略图
	sum := sha1.Sum([]byte(fmt.Sprintf("%s|%d|%d", filename, info.Size(), info.ModTime().Unix())))
	cacheDir := filepath.Join(filepath.Dir(cfg.DBPath), "thumbnails")
	cachePath := filepath.Join(cacheDir, hex.EncodeToString(sum[:])+".jpg")

	if _, statErr := os.Stat(cachePath); statErr != nil {
		if mkErr := os.MkdirAll(cacheDir, 0o755); mkErr != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		// -ss 放在 -i 之前是关键帧定位，比解码到目标时间快得多。
		// 前 2 秒常是黑场或片头，先试第 5 秒，失败再回退到第 1 秒。
		var lastErr error
		for _, ss := range []string{"5", "1"} {
			out, runErr := exec.Command(ffmpeg,
				"-ss", ss, "-i", src, "-frames:v", "1",
				"-vf", "scale=320:-2", "-q:v", "4", "-y", cachePath,
			).CombinedOutput()
			if runErr == nil {
				lastErr = nil
				break
			}
			lastErr = fmt.Errorf("%v: %s", runErr, strings.TrimSpace(string(out)))
			os.Remove(cachePath)
		}
		if lastErr != nil {
			log.Printf("生成缩略图失败 %s: %v", filename, lastErr)
			c.Status(http.StatusNotFound)
			return
		}
	}

	c.Header("Cache-Control", "private, max-age=86400")
	c.File(cachePath)
}

func DeleteFile(c *gin.Context) {
	filename := c.Param("name")
	if filename == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "文件名不能为空"})
		return
	}

	// 防止路径遍历
	filename = filepath.Base(filename)

	cfg := config.Load()
	filePath := filepath.Join(cfg.DownloadDir, filename)

	if err := os.Remove(filePath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

type BatchDeleteFilesRequest struct {
	Names []string `json:"names" binding:"required,min=1"`
}

// BatchDeleteFiles 批量删除文件。
// 使用 DELETE /files + JSON body，避免与 /files/:name 产生路由冲突。
// 前端此前是 N 次串行请求，50 个文件就要 50 次往返。
func BatchDeleteFiles(c *gin.Context) {
	var req BatchDeleteFilesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}

	cfg := config.Load()
	deleted := make([]string, 0, len(req.Names))
	failed := make([]string, 0)

	for _, name := range req.Names {
		if name == "" {
			continue
		}
		// 防止路径遍历
		safeName := filepath.Base(name)
		if err := os.Remove(filepath.Join(cfg.DownloadDir, safeName)); err != nil {
			failed = append(failed, name)
			continue
		}
		deleted = append(deleted, name)
	}

	c.JSON(http.StatusOK, gin.H{
		"deleted": deleted,
		"failed":  failed,
	})
}

// Health 健康检查：无需鉴权，供容器 healthcheck 使用。
// 不能复用 /api/user，因为它需要登录、未登录返回 401，会让健康检查永远失败。
func Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func getVideoMimeType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	if mimeType, ok := videoMimeTypes[ext]; ok {
		return mimeType
	}
	return "application/octet-stream"
}
