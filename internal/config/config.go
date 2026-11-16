package config

import (
	"crypto/rand"
	"fmt"
	"log"
	"math/big"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port            int
	DBPath          string
	DownloadDir     string
	AdminPassword   string
	BinDir          string
	AllowOrigins    string
	AllowInsecure   bool // 是否允许非HTTPS环境（开发模式）
	DownloadTimeout int  // 下载超时时间（秒），0表示不限制
	MaxConcurrent   int  // 同时下载的任务数上限（队列容量）
}

// generateAdminPassword 在管理员未通过 ADMIN_PASSWORD 指定密码时生成一个随机密码。
//
// 历史上这里用的是硬编码的 "admin123"。合规审核指出「应用本地明文存储密码」后复查
// 发现：数据库里存的确实是 bcrypt 哈希，但未设置环境变量时会退化成这个公开的弱密码，
// 等于任何人都能用 admin/admin123 登录。现在改为随机生成并打印到启动日志。
func generateAdminPassword() string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"
	const length = 24
	buf := make([]byte, length)
	for i := range buf {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			// 随机数不可用时宁可让进程失败，也不要回退到可预测的密码
			log.Fatalf("生成随机管理员密码失败: %v", err)
		}
		buf[i] = alphabet[n.Int64()]
	}
	return string(buf)
}

func Load() *Config {
	port, _ := strconv.Atoi(os.Getenv("PORT"))
	if port == 0 {
		port = 8080
	}

	downloadDir := os.Getenv("DOWNLOAD_DIR")
	if downloadDir == "" {
		downloadDir = "./downloads"
	}

	adminPassword := os.Getenv("ADMIN_PASSWORD")
	if adminPassword == "" {
		adminPassword = generateAdminPassword()
		// 只在数据库还没有管理员时才会被使用；已有账号时这段密码会被忽略。
		log.Printf("⚠️  未设置环境变量 ADMIN_PASSWORD，已生成随机管理员密码（仅首次初始化生效）: %s", adminPassword)
		log.Printf("   要固定密码请在启动参数或 docker-compose 中设置 ADMIN_PASSWORD")
	}

	binDir := os.Getenv("BIN_DIR")
	if binDir == "" {
		binDir = "./bin"
	}

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./db/data.db"
	}

	allowOrigins := os.Getenv("ALLOW_ORIGINS")
	if allowOrigins == "" {
		allowOrigins = "http://localhost:8080,http://127.0.0.1:8080"
	}

	allowInsecure, _ := strconv.ParseBool(os.Getenv("ALLOW_INSECURE"))

	downloadTimeout, _ := strconv.Atoi(os.Getenv("DOWNLOAD_TIMEOUT"))
	if downloadTimeout <= 0 {
		downloadTimeout = 0 // 0 表示不限制超时
	}

	maxConcurrent, _ := strconv.Atoi(os.Getenv("MAX_CONCURRENT_TASKS"))
	if maxConcurrent <= 0 {
		maxConcurrent = 3 // 单个任务已经开 32 线程，3 个并发在 NAS 上比较均衡
	}

	return &Config{
		Port:            port,
		DBPath:          dbPath,
		DownloadDir:     downloadDir,
		AdminPassword:   adminPassword,
		BinDir:          binDir,
		AllowOrigins:    allowOrigins,
		AllowInsecure:   allowInsecure,
		DownloadTimeout: downloadTimeout,
		MaxConcurrent:   maxConcurrent,
	}
}

func (c *Config) GetN_m3u8DLREPath() string {
	return fmt.Sprintf("%s/N_m3u8DL-RE", c.BinDir)
}

func (c *Config) GetFFmpegPath() string {
	return fmt.Sprintf("%s/ffmpeg", c.BinDir)
}

func (c *Config) GetMp4decryptPath() string {
	return fmt.Sprintf("%s/mp4decrypt", c.BinDir)
}

// GetDownloadTimeout 返回下载超时时间
func (c *Config) GetDownloadTimeout() time.Duration {
	if c.DownloadTimeout <= 0 {
		return 0 // 不限制超时
	}
	return time.Duration(c.DownloadTimeout) * time.Second
}
