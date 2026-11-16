# 松鼠下载器 · API 文档（面向 Agent）

这份文档面向**自动化调用方**（AI Agent、爬虫、工作流工具如 Hermes / WorkBuddy 等），
说明如何通过 HTTP 接口把视频下载任务交给松鼠下载器执行，并取回文件。

> 人类用户请参考 [使用教程](https://panoptes88.github.io/squirrel-downloader/)。

---

## 概览

| 项目 | 值 |
| --- | --- |
| Base URL | `http://<NAS_IP>:<PORT>` |
| UPK 安装默认端口 | `21808` |
| Docker 安装默认端口 | `8080` |
| 数据格式 | JSON（UTF-8） |
| 鉴权方式 | Cookie `auth_token` |
| 是否有 OpenAPI 文档 | 否，以本文为准 |

**核心流程只有四步**：

```
GET  /api/auth/encryption-key  取公钥
POST /api/auth/login-encrypted  密文提交，拿 Cookie
POST /api/tasks          提交下载链接
GET  /api/tasks/{id}     轮询直到 completed
GET  /api/files/download 取回文件
```

一次典型下载（30 分钟的视频）在 NAS 上耗时 **5-15 秒**，取决于源站速度。

---

## 一、鉴权

### 1.0 推荐：用 API 令牌（无需加密）

**Agent、脚本、工作流工具不需要实现加密**，在应用「设置 → API 令牌」里创建一个令牌即可。

创建方式：登录 Web 界面 → 右上角用户名 → 「API 令牌」→ 填名称、选有效期 → 创建。
令牌只在创建时显示一次，请立即复制。

```bash
curl -H "Authorization: Bearer sq_AbCdEf..." http://192.168.1.50:8080/api/tasks
```

| 项目 | 说明 |
| --- | --- |
| 有效期 | 1 天 / 1 星期 / 1 个月 / 3 个月 / 6 个月 / 1 年 / 永久 |
| 存储 | 服务端只存 SHA-256 哈希，明文不可找回 |
| 撤销 | 设置页随时撤销，立即失效 |
| 权限 | 只能调用业务接口（任务、文件、设置）；**不能**改密、注销、创建新令牌 |
| 数量上限 | 未撤销的令牌最多 **5 个**，超出会拒绝创建 |

请求头与登录会话完全一致，都是 `Authorization: Bearer <token>`，
服务端按前缀 `sq_` 自动区分会话令牌和 API 令牌。

> 需要自己实现加密的场景（例如做一些特殊的集成），见下面的密文登录章节；
> 日常调用用 API 令牌就够了。

### 1.1 登录（密文提交）

登录分两步：先取公钥与一次性 challenge，再把凭据加密后提交。

**明文登录接口 `POST /api/auth/login` 已在 1.0.0.0018 移除**（合规要求：HTTP 下密码不得明文传输）。

#### 第一步：取公钥

```http
GET /api/auth/encryption-key
```

```json
{
  "key_id": "7KUUu0R-qrPPcheZ",
  "algorithm": "RSA-OAEP-256+A256GCM",
  "public_key": "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkq...",
  "challenge": "tOlVtvFv7hSdEbhJ5cRIaVO4...",
  "expires_at": "2026-11-17T12:00:00+08:00",
  "expires_in": 120
}
```

`challenge` **单次有效**，2 分钟过期，重放会被拒绝。

#### 第二步：加密并提交

算法固定为 `RSA-OAEP-256+A256GCM`：

1. 随机生成 32 字节 AES-256 会话密钥
2. 用 RSA 公钥以 **RSA-OAEP(SHA-256)** 加密这把密钥 → `encrypted_key`
3. 随机生成 12 字节 IV
4. 用 AES-256-GCM 加密 `{"username","password","challenge"}` → `ciphertext`
   （GCM 的认证标签**拼在密文末尾**，与 Go 的 `cipher.GCM.Open` 一致）

```http
POST /api/auth/login-encrypted
Content-Type: application/json

{
  "key_id": "7KUUu0R-qrPPcheZ",
  "algorithm": "RSA-OAEP-256+A256GCM",
  "encrypted_key": "<base64>",
  "iv": "<base64>",
  "ciphertext": "<base64>"
}
```

响应（HTTP 200）：

```
Set-Cookie: auth_token=515ece26-...; Path=/; Max-Age=86400; HttpOnly
```

```json
{"id": 1, "username": "admin"}
```

**Cookie 有效期 24 小时**，之后需要重新登录。

#### 客户端实现

| 环境 | 用什么 | 说明 |
| --- | --- | --- |
| 浏览器网页 | node-forge | 局域网 HTTP 下 `crypto.subtle` 不可用（非安全上下文） |
| 浏览器插件 | 原生 WebCrypto | `chrome-extension://` 属于安全上下文，无需额外依赖 |
| Python / Node 脚本 | 任意 RSA-OAEP + AES-GCM 库 | 见下方示例 |

改密同理：`POST /api/auth/change-password-encrypted`，明文体为
`{"old_password","new_password","challenge"}`。明文改密接口同样已移除。

### 1.2 两种携带方式：Cookie 与 Bearer

鉴权中间件**同时**接受两种方式，取第一个非空的：

| 方式 | 请求头 | 适用 |
| --- | --- | --- |
| Cookie | `Cookie: auth_token=<token>` | 浏览器 / Web 界面 |
| Bearer | `Authorization: Bearer <token>` | **Agent / 脚本 / 自动化工具** |

登录接口在响应体里也会返回 `access_token`，直接拿它放进 `Authorization` 头即可，
**不需要维护 Cookie Jar**：

```json
{
  "id": 1,
  "username": "admin",
  "access_token": "515ece26-6b67-49c3-b62c-5b981e8cb211",
  "expires_in": 86400
}
```

登录必须走密文。完整的加密与登录流程见下方「九、完整示例（Python）」。

`Bearer` 的 scheme 大小写不敏感（`bearer` 也接受）。

> Cookie 和 Bearer 指向**同一个会话 token**。退出登录（`POST /api/auth/logout`）
> 或修改密码后，两者同时失效。

### 1.3 退出

```http
POST /api/auth/logout
```

### 1.3 现成脚本：一条命令拿 token

不想自己写加密逻辑的话，仓库里有现成的 `scripts/get-token.py`。

**依赖只有一个**：`cryptography`（预编译 wheel，实测干净环境 34 秒装完，不需要编译器，约 16MB）

```bash
pip install cryptography

# 直接输出 access_token
./scripts/get-token.py --base http://192.168.1.50:8080 --password '你的密码'

# 用环境变量传密码，避免进命令历史
SQ_PASSWORD='你的密码' ./scripts/get-token.py --base http://192.168.1.50:8080

# 拿到 token 后用 Bearer 调接口
TOKEN=$(./scripts/get-token.py --base http://192.168.1.50:8080)
curl -H "Authorization: Bearer $TOKEN" http://192.168.1.50:8080/api/tasks

# 也可以直接拿会话 Cookie
./scripts/get-token.py --base http://192.168.1.50:8080 --format cookie

# 或者一次拿全（JSON）
./scripts/get-token.py --base http://192.168.1.50:8080 --format json
→ {"access_token":"...","cookie":"auth_token=..."}
```

退出码：`0` 成功 / `1` 服务端拒绝（密码错等）/ `2` 网络或加密失败。

### 1.4 未鉴权的响应

```json
HTTP 401
{"error": "请先登录"}
```

会话过期时是 `{"error": "登录已过期，请重新登录"}`，两者都应该触发**重新登录**。

---

## 二、提交下载任务

```http
POST /api/tasks
Content-Type: application/json
Cookie: auth_token=...

{
  "url": "https://example.com/video/index.m3u8",
  "output_name": "我的视频",
  "auto_select": true
}
```

### 请求字段

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `url` | string | **是** | m3u8 / mpd 或直链地址 |
| `output_name` | string | 否 | 输出文件名（不含扩展名）。留空会自动从 URL 推导 |
| `auto_select` | bool | 否 | **默认 true**，自动选最佳音视频流。不要传 false——容器里没有 TTY，多码率流会弹交互菜单导致任务卡在 0% |
| `headers` | string | 否 | 自定义请求头，**换行分隔**，如 `Cookie: a=1; b=2\nUser-Agent: Mozilla/5.0` |
| `key` | string | 否 | AES-128 解密密钥（16 字节 hex） |
| `decryption_engine` | string | 否 | `MP4DECRYPT`（默认）/ `SHAKA_PACKAGER` / `FFMPEG` |
| `concurrent_download` | bool | 否 | 并行下载音视频并合成为 mp4 |
| `thread_count` | int | 否 | 分片并发数，默认 32 |
| `custom_proxy` | string | 否 | 代理地址，如 `http://192.168.1.2:7890` |
| `del_after_done` | bool | 否 | 合并后删除分片，默认 true |

### 响应

返回完整的任务对象（HTTP 200）：

```json
{
  "id": 1,
  "url": "https://example.com/video/index.m3u8",
  "status": "pending",
  "progress": 0,
  "output_name": "我的视频",
  "total_size": "",
  "thread_count": 32,
  "auto_select": true,
  "created_at": "2026-11-13T22:15:54.329250242+08:00"
}
```

**记住 `id`**，后续轮询和取文件都用它。

### 批量提交

```http
POST /api/tasks/batch
Authorization: Bearer <token>
Content-Type: application/json

[
  {"url": "https://example.com/a.m3u8", "output_name": "视频A", "auto_select": true},
  {"url": "https://example.com/b.m3u8", "output_name": "视频B", "auto_select": true}
]
```

也接受 `{"tasks": [...]}` 的包裹写法。

每个元素的字段与单条创建**完全一致**。

### 响应：部分成功

HTTP 200（只要有一条成功）：

```json
{
  "created": [
    {"id": 4, "output_name": "视频A", "status": "pending", "...": "..."},
    {"id": 5, "output_name": "视频B", "status": "pending", "...": "..."}
  ],
  "errors": [
    {"index": 2, "url": "", "error": "url 不能为空"}
  ]
}
```

| 字段 | 说明 |
| --- | --- |
| `created` | 成功入队的任务对象数组 |
| `errors` | 失败的条目，`index` 对应请求数组里的下标 |

**全部失败**时返回 HTTP 400，响应体结构相同（`created` 为空数组）。

### 限制

| 项目 | 值 |
| --- | --- |
| 单次上限 | **50 条**，超过返回 400 |
| 空数组 | 400 |

### 注意：不要用重复的 `output_name`

批量和单条任务共用输出目录。两个任务生成同名文件会互相覆盖，
后启动的那个大概率以 `interrupted` 结束（错误信息「下载进程已中断」）。

如果确实要重新下载同一个视频，先改 `output_name`，或删掉旧文件。

---

## 三、轮询任务状态

```http
GET /api/tasks/{id}
Cookie: auth_token=...
```

### 任务对象字段

| 字段 | 说明 |
| --- | --- |
| `id` | 任务 ID |
| `status` | 见下方状态机 |
| `progress` | 进度百分比，0-100 |
| `total_size` | 产物大小，**完成后才有值**，如 `"48.88MB"` |
| `output_name` | 输出文件名（不含扩展名） |
| `speed` | 下载速度，仅下载中有效 |
| `error_msg` | 失败原因，仅失败时存在 |
| `created_at` / `finished_at` | 时间戳（RFC3339，含时区） |

> 完成后 `speed` 和 `downloaded_size` 会被清空，判断是否成功只看 `status`。

### 状态机

```
pending ──▶ downloading ──▶ completed
   │             │
   │             └────────▶ failed
   └──▶ interrupted（进程重启导致）
```

| 状态 | 含义 | 建议动作 |
| --- | --- | --- |
| `pending` | 排队中（并发已满） | 继续轮询 |
| `downloading` | 下载中 | 继续轮询 |
| `completed` | 成功 | 取文件 |
| `failed` | 失败，看 `error_msg` | 可重试 |
| `interrupted` | 被中断（服务重启） | 调 retry |

### 建议的轮询节奏

```
前 10 秒：每 1 秒一次
之后：    每 3 秒一次
上限：    10 分钟（超时视为失败，调 cancel）
```

短任务（几分钟的视频）通常 30 秒内完成；一小时的视频可能 2-5 分钟。

---

## 四、取回文件

```http
GET /api/files/download?name=<URL 编码后的文件名>
Cookie: auth_token=...
```

- `name` 必须是**带扩展名的完整文件名**，如 `我的视频.mp4`（中文要 URL 编码）
- 支持 `Range` 请求（断点续传）
- 响应带 `Content-Disposition: attachment`

```bash
# 从任务对象拼出文件名
NAME="我的视频.mp4"
curl -H "Cookie: auth_token=$TOKEN" -OJ \
  "http://NAS:8080/api/files/download?name=$(python3 -c "import urllib.parse,sys;print(urllib.parse.quote(sys.argv[1]))" "$NAME")"
```

---

## 五、文件列表

```http
GET /api/files
Cookie: auth_token=...
```

返回数组，**只包含视频和字幕文件**：

```json
[
  {
    "name": "我的视频.mp4",
    "size": 51259492,
    "ext": "MP4",
    "kind": "video",
    "modTime": "2026-11-13 22:16:01",
    "createTime": "2026-11-13 22:16:01",
    "accessTime": "2026-11-13 22:16:01"
  }
]
```

| 字段 | 说明 |
| --- | --- |
| `kind` | `video` / `subtitle` / 其他 |
| `ext` | 大写扩展名 |
| `size` | 字节数 |
| 时间字段 | 格式 `YYYY-MM-DD HH:mm:ss`，服务器本地时区 |

---

## 六、其他接口

### 6.1 任务列表

```http
GET /api/tasks
GET /api/tasks?status=completed
```

返回数组（不分页）。`status` 可选：`pending` / `downloading` / `completed` / `failed` / `interrupted`。

### 6.2 任务日志

```http
GET /api/tasks/{id}/log
```

```json
{"log": "22:15:56.137 INFO : Output is redirected...\n..."}
```

日志可能很长（数万行），**只在排查失败时读**。内容超过阈值时中间部分会被省略。

### 6.3 取消 / 重试 / 删除

```http
POST   /api/tasks/{id}/cancel     # 取消，进程被 kill
POST   /api/tasks/{id}/retry      # 重试失败或中断的任务
DELETE /api/tasks/{id}            # 删除任务记录（不删文件）
```

### 6.4 删除文件

```http
DELETE /api/files/{文件名}
```

```bash
curl -X DELETE -H "Cookie: auth_token=$TOKEN" \
  "http://NAS:8080/api/files/$(python3 -c "import urllib.parse,sys;print(urllib.parse.quote(sys.argv[1]))" '我的视频.mp4')"
```

### 6.5 设置

```http
GET /api/settings
PUT /api/settings
```

```json
{"max_concurrent_tasks": 3}
```

`max_concurrent_tasks` 范围 1-10。Agent 批量提交任务时，可以考虑临时调大这个值，
**但要注意 NAS 的带宽和磁盘 IO**，不建议超过 5。

### 6.6 下载方案（按域名预设请求头）

```http
GET  /api/profiles                    # 列表
POST /api/profiles                    # 创建
GET  /api/profiles/by-domain?domain=example.com
```

如果某个站点每次都需要相同的 Cookie/UA，可以存成方案，之后提交任务时不传 `headers`，
服务端会按域名自动匹配。

### 6.7 健康检查（无需鉴权）

```http
GET /api/health
```

```json
{"status": "ok"}
```

用于探活，**不要**用它在轮询里判断服务状态。

---

## 七、错误处理

所有错误都是 JSON：

```json
{"error": "人类可读的中文错误信息"}
```

| HTTP | 场景 | 处理 |
| --- | --- | --- |
| 400 | 参数错误（如 `url` 为空） | 修正参数，不要重试 |
| 401 | 未登录 / 会话过期 | **重新登录**后重试 |
| 404 | 任务或文件不存在 | 检查 ID / 文件名 |
| 500 | 服务端异常 | 指数退避重试 2-3 次 |

**不要对 400 重试**——那是请求本身的问题。

---

## 八、给 Agent 的实践建议

### 8.1 一次会话复用 Cookie

登录一次，把 `auth_token` 存起来，后续所有请求复用。24 小时内不用重复登录。

### 8.2 提交任务前先确认服务可达

```python
requests.get(f"{base}/api/health", timeout=5)
```

### 8.3 任务提交后立刻记录 ID

`POST /api/tasks` 返回的 `id` 是唯一凭据。如果请求超时但服务端其实创建成功了，
可以用 `GET /api/tasks` 查最近的任务，比对 `url` 和 `created_at`，避免重复下载。

### 8.4 并发控制

服务端默认同时跑 3 个任务。**提交更多也不会失败**，只会排队。
但如果一次性提交 50 个，Agent 自己的轮询逻辑要能处理大量 `pending` 状态。

### 8.5 需要登录态的站点

很多视频站需要 Cookie 才能拉到分片。做法：

1. 从浏览器（或插件）导出该站点的 Cookie
2. 放进 `headers` 字段，换行分隔多个头

```json
{
  "url": "https://xxx/index.m3u8",
  "headers": "Cookie: session=abc123; uid=456\nUser-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64)"
}
```

**换行用 `\n`**，不要用分号拼接——Cookie 值本身就可能含分号。

### 8.6 需要代理时

`custom_proxy` 填 NAS 能访问到的代理地址。注意是 **NAS 的视角**，
不是 Agent 所在机器的视角。

### 8.7 失败重试

```python
if task["status"] in ("failed", "interrupted"):
    # 先看错误
    log = get(f"/api/tasks/{id}/log")["log"]
    if "404" in log or "not found" in log.lower():
        pass  # 链接失效，重试无意义
    else:
        post(f"/api/tasks/{id}/retry")
```

---

## 九、完整示例（Python）

以下脚本经过实测，可直接运行：

```python
import time
import urllib.parse

import requests

BASE = "http://192.168.1.50:8080"      # 改成你的 NAS 地址
USER = "admin"
PASSWORD = "你的密码"


def build_encrypted_body(session, base, fields):
    """按服务端要求加密凭据：RSA-OAEP(SHA-256) 包裹 AES-256-GCM 会话密钥。

    依赖 `cryptography`：pip install cryptography requests
    """
    import base64
    import json
    import os

    from cryptography.hazmat.primitives import hashes, serialization
    from cryptography.hazmat.primitives.asymmetric import padding
    from cryptography.hazmat.primitives.ciphers.aead import AESGCM

    key = session.get(f"{base}/api/auth/encryption-key", timeout=10).json()
    public_key = serialization.load_pem_public_key(key["public_key"].encode())

    aes_key = AESGCM.generate_key(bit_length=256)
    encrypted_key = public_key.encrypt(
        aes_key,
        padding.OAEP(mgf=padding.MGF1(hashes.SHA256()),
                     algorithm=hashes.SHA256(), label=None),
    )

    iv = os.urandom(12)
    plaintext = json.dumps({**fields, "challenge": key["challenge"]}).encode("utf-8")
    # AESGCM.encrypt 的返回值已包含认证标签，正好对应 Go 的 cipher.GCM.Open
    ciphertext = AESGCM(aes_key).encrypt(iv, plaintext, None)

    return {
        "key_id": key["key_id"],
        "algorithm": key["algorithm"],
        "encrypted_key": base64.b64encode(encrypted_key).decode(),
        "iv": base64.b64encode(iv).decode(),
        "ciphertext": base64.b64encode(ciphertext).decode(),
    }


class Squirrel:
    def __init__(self, base, user, password):
        self.base = base.rstrip("/")
        self.s = requests.Session()
        body = build_encrypted_body(self.s, self.base,
                                    {"username": user, "password": password})
        r = self.s.post(f"{self.base}/api/auth/login-encrypted", json=body, timeout=10)
        r.raise_for_status()

    def create_task(self, url, output_name="", **kwargs):
        payload = {"url": url, "auto_select": True}
        if output_name:
            payload["output_name"] = output_name
        payload.update(kwargs)
        r = self.s.post(f"{self.base}/api/tasks", json=payload, timeout=30)
        r.raise_for_status()
        return r.json()

    def wait(self, task_id, timeout=600, interval=1.0, max_interval=3.0):
        """轮询直到任务结束，返回最终任务对象"""
        deadline = time.time() + timeout
        while time.time() < deadline:
            r = self.s.get(f"{self.base}/api/tasks/{task_id}", timeout=10)
            r.raise_for_status()
            task = r.json()
            if task["status"] in ("completed", "failed", "interrupted"):
                return task
            print(f"  {task['status']} {task['progress']}%")
            time.sleep(interval)
            interval = min(interval + 0.5, max_interval)
        raise TimeoutError(f"任务 {task_id} 超过 {timeout} 秒未完成")

    def download(self, filename, dest=None):
        """按文件名下载到本地"""
        name = urllib.parse.quote(filename)
        with self.s.get(f"{self.base}/api/files/download?name={name}",
                        stream=True, timeout=300) as r:
            r.raise_for_status()
            dest = dest or filename
            with open(dest, "wb") as f:
                for chunk in r.iter_content(1 << 16):
                    f.write(chunk)
            return dest

    def log(self, task_id):
        return self.s.get(f"{self.base}/api/tasks/{task_id}/log", timeout=30).json()["log"]


if __name__ == "__main__":
    api = Squirrel(BASE, USER, PASSWORD)

    task = api.create_task(
        url="https://devstreaming-cdn.apple.com/videos/streaming/examples/bipbop_4x3/gear1/prog_index.m3u8",
        output_name="测试视频",
    )
    print(f"已提交任务 {task['id']}")

    final = api.wait(task["id"])
    print(f"结果: {final['status']}  大小: {final.get('total_size', '-')}")

    if final["status"] == "completed":
        path = api.download(f"{final['output_name']}.mp4")
        print(f"已保存到 {path}")
    else:
        print(api.log(task["id"])[-2000:])
```

运行输出：

```
已提交任务 1
  pending 0%
  downloading 94%
结果: completed  大小: 48.88MB
已保存到 测试视频.mp4
```

---

## 十、快速验证

```bash
BASE=http://192.168.1.50:8080
TOKEN=sq_你在设置页创建的令牌

# 1. 提交任务
curl -s -H "Authorization: Bearer $TOKEN" -X POST $BASE/api/tasks \
  -H 'Content-Type: application/json' \
  -d '{"url":"https://example.com/index.m3u8","output_name":"测试","auto_select":true}'

# 2. 轮询（假设返回的 id=1）
while true; do
  S=$(curl -s -H "Authorization: Bearer $TOKEN" $BASE/api/tasks/1 \
    | python3 -c 'import sys,json;d=json.load(sys.stdin);print(d["status"],d["progress"],d.get("total_size",""))')
  echo "$S"
  case "$S" in completed*|failed*|interrupted*) break;; esac
  sleep 2
done

# 3. 下载
curl -H "Authorization: Bearer $TOKEN" -OJ \
  "$BASE/api/files/download?name=%E6%B5%8B%E8%AF%95.mp4"
```

---

## 附：接口一览

| 方法 | 路径 | 鉴权 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/health` | 否 | 健康检查 |
| GET | `/api/auth/encryption-key` | 否 | 取加密公钥与 challenge |
| POST | `/api/auth/login-encrypted` | 否 | **密文登录** |
| POST | `/api/auth/logout` | 否 | 退出 |
| GET | `/api/tokens` | 是 | 令牌列表（仅会话）
| POST | `/api/tokens` | 是 | **创建令牌**（仅会话）
| DELETE | `/api/tokens/{id}` | 是 | 撤销令牌（仅会话）
| POST | `/api/auth/change-password-encrypted` | 是 | **密文修改密码** |
| GET | `/api/user` | 是 | 当前用户 |
| GET | `/api/settings` | 是 | 读取设置 |
| PUT | `/api/settings` | 是 | 修改设置 |
| GET | `/api/tasks` | 是 | 任务列表 |
| POST | `/api/tasks` | 是 | **创建下载任务** |
| POST | `/api/tasks/batch` | 是 | **批量创建（≤50）** |
| DELETE | `/api/tasks` | 是 | 批量删除任务 |
| GET | `/api/tasks/{id}` | 是 | **查询任务状态** |
| DELETE | `/api/tasks/{id}` | 是 | 删除任务 |
| POST | `/api/tasks/{id}/cancel` | 是 | 取消任务 |
| POST | `/api/tasks/{id}/retry` | 是 | 重试任务 |
| GET | `/api/tasks/{id}/log` | 是 | 任务日志 |
| POST | `/api/tasks/{id}/save-as-profile` | 是 | 存为下载方案 |
| GET | `/api/files` | 是 | 文件列表 |
| GET | `/api/files/download` | 是 | **下载文件** |
| GET | `/api/files/thumbnail` | 是 | 视频缩略图 |
| DELETE | `/api/files/{name}` | 是 | 删除文件 |
| DELETE | `/api/files` | 是 | 批量删除文件 |
| GET | `/api/profiles` | 是 | 方案列表 |
| POST | `/api/profiles` | 是 | 创建方案 |
| GET | `/api/profiles/{id}` | 是 | 方案详情 |
| PUT | `/api/profiles/{id}` | 是 | 更新方案 |
| DELETE | `/api/profiles/{id}` | 是 | 删除方案 |
| GET | `/api/profiles/by-domain` | 是 | 按域名查方案 |

---

## 附：已知限制

1. **登录必须加密**：明文接口已移除，调用方需要实现 RSA-OAEP + AES-GCM
2. **没有回调/Webhook**：只能轮询 `GET /api/tasks/{id}`
2. **任务列表不分页**：`GET /api/tasks` 返回全部，任务很多时响应会比较大
3. **日志接口没有 `tail` 参数**：返回完整日志，长任务时很大
4. **批量上限 50 条**：超过需要分批提交

这几项如果对自动化场景比较重要，可以提 issue。

---

## 更新记录

| 版本 | 变更 |
| --- | --- |
| 1.0.0.0017 | 初始版本 |
| 1.1.0（未发布） | 新增 `Authorization: Bearer`；新增 `POST /api/tasks/batch` |
