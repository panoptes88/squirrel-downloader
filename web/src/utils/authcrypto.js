/**
 * 登录 / 改密的密文传输（前端）。
 *
 * 为什么不用 WebCrypto：
 *   `crypto.subtle` 只在 HTTPS 或 localhost 下可用。本应用默认部署在局域网
 *   HTTP 上，恰恰是 `crypto.subtle === undefined` 的场景，所以这里用纯 JS 的
 *   node-forge 实现同样的算法，保证 HTTP 下也能加密。
 *
 * 与后端 internal/service/authcrypto.go 对应：
 *   1. GET /api/auth/encryption-key 拿 RSA 公钥 + 一次性 challenge
 *   2. 随机生成 AES-256 会话密钥
 *   3. RSA-OAEP-SHA256 加密会话密钥
 *   4. AES-256-GCM 加密 { ...fields, challenge }
 */
import forge from 'node-forge'
import { get } from '../api'

const ALGORITHM = 'RSA-OAEP-256+A256GCM'

/** 取公钥与一次性 challenge。Cache-Control: no-store，不复用。 */
export async function fetchEncryptionKey() {
  return get('/auth/encryption-key')
}

function pemToPublicKey(pem) {
  return forge.pki.publicKeyFromPem(pem)
}

function randomBytes(n) {
  return forge.random.getBytesSync(n)
}

/**
 * 加密凭据。
 *
 * @param {object} fields  需要加密的字段，如 {username, password}
 * @param {object} keyInfo fetchEncryptionKey() 的返回值
 * @returns {object} 可直接 POST 给 /auth/login-encrypted 的密文结构
 */
export function encryptAuthPayload(fields, keyInfo) {
  const publicKey = pemToPublicKey(keyInfo.public_key)

  // 1) 随机 AES-256 会话密钥
  const aesKey = randomBytes(32)

  // 2) 用 RSA-OAEP-SHA256 加密会话密钥
  const encryptedKey = publicKey.encrypt(aesKey, 'RSA-OAEP', {
    md: forge.md.sha256.create(),
    mgf1: { md: forge.md.sha256.create() }
  })

  // 3) AES-256-GCM 加密明文
  const iv = randomBytes(12)
  const plaintext = JSON.stringify({ ...fields, challenge: keyInfo.challenge })

  const cipher = forge.cipher.createCipher('AES-GCM', aesKey)
  cipher.start({ iv })
  cipher.update(forge.util.createBuffer(forge.util.encodeUtf8(plaintext)))
  cipher.finish()

  return {
    key_id: keyInfo.key_id,
    algorithm: ALGORITHM,
    encrypted_key: forge.util.encode64(encryptedKey),
    iv: forge.util.encode64(iv),
    // GCM 的认证 tag 需要拼在密文末尾，Go 的 cipher.GCM.Open 接受这种格式
    ciphertext: forge.util.encode64(cipher.output.getBytes() + cipher.mode.tag.getBytes())
  }
}

/**
 * 一次性完成「取公钥 → 加密」，调用方直接拿到密文结构。
 */
export async function buildEncryptedPayload(fields) {
  const keyInfo = await fetchEncryptionKey()
  if (!keyInfo?.public_key || !keyInfo?.challenge) {
    throw new Error('服务端未提供加密公钥')
  }
  return encryptAuthPayload(fields, keyInfo)
}

export { ALGORITHM }
