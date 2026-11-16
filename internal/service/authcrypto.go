package service

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"sync"
	"time"
)

// 登录 / 改密的密文传输。
//
// 背景：应用经常部署在局域网里，走的是明文 HTTP。合规审核要求「应使用 HTTPS
// 或密文传输应用个人信息」——HTTPS 需要用户自己配证书和反向代理，所以这里在
// 应用层再兜一层：
//
//	GET  /api/auth/encryption-key  → 一次性 challenge + RSA 公钥
//	POST /api/auth/login-encrypted → 提交密文，服务端解密后再校验
//
// 方案与 KOLFlow 一致：
//
//  1. 服务端启动时生成一对 RSA-2048 密钥（进程内，重启即轮换）；
//  2. 客户端每次提交前先取 challenge，并用它作为明文的一部分，
//     服务端消费后立即作废，防止重放；
//  3. 客户端随机生成 AES-256 会话密钥，用 RSA-OAEP-SHA256 加密这把密钥，
//     再用 AES-256-GCM 加密真正的凭据——RSA 只加密 32 字节，不拖慢性能；
//  4. GCM 自带完整性校验，密文被篡改会直接解密失败。
//
// 明文 HTTP 下抓包只能看到密文与一次性 challenge，无法重放，也无法还原密码。

const (
	// AuthEncryptionAlgorithm 是客户端需要实现的算法标识，大小写不敏感。
	AuthEncryptionAlgorithm = "RSA-OAEP-256+A256GCM"

	// challengeTTL 是 challenge 的有效期。
	challengeTTL = 2 * time.Minute

	// maxPendingChallenges 限制未消费的 challenge 数量，防止被刷爆内存。
	maxPendingChallenges = 10000

	// 密文字段长度上限，避免超大请求体。
	maxEncryptedFieldBytes = 4096
	maxPlaintextBytes      = 8192
)

var (
	// ErrChallengeInvalid 表示 challenge 不存在、已用过或已过期。
	ErrChallengeInvalid = errors.New("challenge 无效或已过期，请重试")

	// ErrDecryptFailed 表示密文格式错误或解密失败。
	ErrDecryptFailed = errors.New("密文解析失败")
)

// AuthEncryptionKey 下发给客户端的公钥与挑战值。
type AuthEncryptionKey struct {
	KeyID     string    `json:"key_id"`
	Algorithm string    `json:"algorithm"`
	PublicKey string    `json:"public_key"` // PEM 编码的 SPKI 公钥
	Challenge string    `json:"challenge"`  // base64url，一次性
	ExpiresAt time.Time `json:"expires_at"`
	ExpiresIn int       `json:"expires_in"` // 秒
}

// EncryptedPayload 是客户端提交的密文结构。
type EncryptedPayload struct {
	KeyID        string `json:"key_id"`
	Algorithm    string `json:"algorithm"`
	EncryptedKey string `json:"encrypted_key"` // RSA-OAEP 加密的 AES 密钥
	IV           string `json:"iv"`            // AES-GCM nonce
	Ciphertext   string `json:"ciphertext"`    // AES-GCM 密文（含 tag）
}

// AuthPlaintext 是解密后得到的明文。
type AuthPlaintext struct {
	Username    string `json:"username,omitempty"`
	Password    string `json:"password,omitempty"`
	OldPassword string `json:"old_password,omitempty"`
	NewPassword string `json:"new_password,omitempty"`
	Challenge   string `json:"challenge"`
	// DeleteFiles 仅注销账号时使用：是否连下载目录里的视频一起删
	DeleteFiles bool `json:"delete_files,omitempty"`
}

type authCrypto struct {
	privateKey *rsa.PrivateKey
	publicPEM  string
	keyID      string

	mu      sync.Mutex
	pending map[string]time.Time // challenge -> 过期时间
}

var (
	authCryptoOnce sync.Once
	authCryptoInst *authCrypto
)

// GetAuthCrypto 返回进程内单例。密钥在首次调用时生成。
func GetAuthCrypto() *authCrypto {
	authCryptoOnce.Do(func() {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			// RSA-2048 生成失败属于极小概率事件，直接 panic 比静默降级更安全：
			// 静默降级会让密文登录不可用却没有任何提示。
			panic(fmt.Sprintf("生成 RSA 密钥失败: %v", err))
		}
		der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
		if err != nil {
			panic(fmt.Sprintf("导出公钥失败: %v", err))
		}
		sum := sha256.Sum256(der)
		authCryptoInst = &authCrypto{
			privateKey: key,
			publicPEM:  string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})),
			keyID:      base64.RawURLEncoding.EncodeToString(sum[:12]),
			pending:    make(map[string]time.Time),
		}
	})
	return authCryptoInst
}

// IssueKey 下发公钥并登记一个新的 challenge。
func (a *authCrypto) IssueKey() AuthEncryptionKey {
	challenge := randomToken(32)
	expiresAt := time.Now().Add(challengeTTL)

	a.mu.Lock()
	a.cleanupLocked(time.Now())
	// 超出上限时丢掉最早登记的一批，保证不会被刷爆内存
	for len(a.pending) >= maxPendingChallenges {
		var oldestKey string
		var oldestTime time.Time
		for k, v := range a.pending {
			if oldestKey == "" || v.Before(oldestTime) {
				oldestKey, oldestTime = k, v
			}
		}
		delete(a.pending, oldestKey)
	}
	a.pending[challenge] = expiresAt
	a.mu.Unlock()

	return AuthEncryptionKey{
		KeyID:     a.keyID,
		Algorithm: AuthEncryptionAlgorithm,
		PublicKey: a.publicPEM,
		Challenge: challenge,
		ExpiresAt: expiresAt,
		ExpiresIn: int(challengeTTL.Seconds()),
	}
}

// Open 解密客户端提交的密文，并消费其中的 challenge。
//
// challenge 一旦提交无论解密是否成功都会作废——否则攻击者可以拿同一个
// challenge 反复试错。
func (a *authCrypto) Open(p *EncryptedPayload) (*AuthPlaintext, error) {
	if p == nil {
		return nil, ErrDecryptFailed
	}
	if p.KeyID != "" && p.KeyID != a.keyID {
		// 服务端重启后密钥会轮换，此时提示客户端重新取公钥
		return nil, errors.New("公钥已失效，请刷新页面后重试")
	}
	if p.Algorithm != "" && !equalFold(p.Algorithm, AuthEncryptionAlgorithm) {
		return nil, fmt.Errorf("不支持的加密算法: %s", p.Algorithm)
	}

	encKey, err := decodeB64(p.EncryptedKey)
	if err != nil || len(encKey) > maxEncryptedFieldBytes {
		return nil, ErrDecryptFailed
	}
	iv, err := decodeB64(p.IV)
	if err != nil || len(iv) > maxEncryptedFieldBytes {
		return nil, ErrDecryptFailed
	}
	ciphertext, err := decodeB64(p.Ciphertext)
	if err != nil || len(ciphertext) > maxEncryptedFieldBytes*2 {
		return nil, ErrDecryptFailed
	}

	// 1) 用 RSA 私钥解出 AES 会话密钥
	aesKey, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, a.privateKey, encKey, nil)
	if err != nil {
		return nil, ErrDecryptFailed
	}
	if len(aesKey) != 32 {
		return nil, ErrDecryptFailed
	}

	// 2) AES-256-GCM 解密
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, ErrDecryptFailed
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, ErrDecryptFailed
	}
	if len(iv) != gcm.NonceSize() {
		return nil, ErrDecryptFailed
	}
	plain, err := gcm.Open(nil, iv, ciphertext, nil)
	if err != nil {
		return nil, ErrDecryptFailed
	}
	if len(plain) > maxPlaintextBytes {
		return nil, ErrDecryptFailed
	}

	var out AuthPlaintext
	if err := json.Unmarshal(plain, &out); err != nil {
		return nil, ErrDecryptFailed
	}

	// 3) 校验并消费 challenge（单次有效）
	if !a.consumeChallenge(out.Challenge) {
		return nil, ErrChallengeInvalid
	}
	return &out, nil
}

func (a *authCrypto) consumeChallenge(challenge string) bool {
	if challenge == "" {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	exp, ok := a.pending[challenge]
	if !ok {
		return false
	}
	delete(a.pending, challenge)
	return time.Now().Before(exp)
}

func (a *authCrypto) cleanupLocked(now time.Time) {
	for k, exp := range a.pending {
		if !now.Before(exp) {
			delete(a.pending, k)
		}
	}
}

func randomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("生成随机数失败: %v", err))
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeB64(s string) ([]byte, error) {
	if s == "" {
		return nil, ErrDecryptFailed
	}
	if b, err := base64.StdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	// 客户端可能用 URL-safe 变体
	return base64.RawURLEncoding.DecodeString(s)
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
