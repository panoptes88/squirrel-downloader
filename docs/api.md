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
POST /api/auth/login     拿 Cookie
POST /api/tasks          提交下载链接
GET  /api/tasks/{id}     轮询直到 completed
GET  /api/files/download 取回文件
```

一次典型下载（30 分钟的视频）在 NAS 上耗时 **5-15 秒**，取决于源站速度。

---

## 一、鉴权

### 1.1 登录

```http
POST /api/auth/login
Content-Type: application/json

{"username": "admin", "password": "你的密码"}
```

响应（HTTP 200）：

```
Set-Cookie: auth_token=515ece26-...; Path=/; Max-Age=86400; HttpOnly
```

```json
{"id": 1, "username": "admin"}
```

**Cookie 有效期 24 小时**，之后需要重新登录。

### 1.2 只有 Cookie，没有 Bearer

鉴权中间件只读取 `auth_token` **Cookie**，**不支持** `Authorization: Bearer <token>`。

所以调用方必须：

- 用带 Cookie Jar 的 HTTP 客户端（Python `requests.Session()`、Node `axios` + `withCredentials`
  或手动解析 `Set-Cookie`）
- 或者手动把 `auth_token` 的值放进 `Cookie` 请求头

```bash
# 手动方式
TOKEN=$(curl -s -i -X POST http://NAS:8080/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"xxx"}' \
  | grep -i '^set-cookie:' | sed 's/.*auth_token=\([^;]*\).*/\1/')

curl -H "Cookie: auth_token=$TOKEN" http://NAS:8080/api/tasks
```

### 1.3 退出

```http
POST /api/auth/logout
```

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


class Squirrel:
    def __init__(self, base, user, password):
        self.base = base.rstrip("/")
        self.s = requests.Session()
        r = self.s.post(f"{self.base}/api/auth/login",
                        json={"username": user, "password": password}, timeout=10)
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

## 十、curl 快速验证

```bash
BASE=http://192.168.1.50:8080
JAR=$(mktemp)

# 1. 登录
curl -s -c $JAR -X POST $BASE/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"你的密码"}'

# 2. 提交任务
curl -s -b $JAR -X POST $BASE/api/tasks \
  -H 'Content-Type: application/json' \
  -d '{"url":"https://example.com/index.m3u8","output_name":"测试","auto_select":true}'

# 3. 轮询（假设返回的 id=1，换成你自己的）
while true; do
  S=$(curl -s -b $JAR $BASE/api/tasks/1 \
    | python3 -c 'import sys,json;d=json.load(sys.stdin);print(d["status"],d["progress"],d.get("total_size",""))')
  echo "$S"
  case "$S" in completed*|failed*|interrupted*) break;; esac
  sleep 2
done

# 4. 下载
curl -b $JAR -OJ "$BASE/api/files/download?name=%E6%B5%8B%E8%AF%95.mp4"

rm -f $JAR
```

---

## 附：接口一览

| 方法 | 路径 | 鉴权 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/health` | 否 | 健康检查 |
| POST | `/api/auth/login` | 否 | 登录 |
| POST | `/api/auth/logout` | 否 | 退出 |
| POST | `/api/auth/change-password` | 是 | 修改密码 |
| GET | `/api/user` | 是 | 当前用户 |
| GET | `/api/settings` | 是 | 读取设置 |
| PUT | `/api/settings` | 是 | 修改设置 |
| GET | `/api/tasks` | 是 | 任务列表 |
| POST | `/api/tasks` | 是 | **创建下载任务** |
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

1. **没有 Bearer Token**：只能走 Cookie，Agent 需要维护 Cookie Jar
2. **没有回调/Webhook**：只能轮询 `GET /api/tasks/{id}`
3. **没有批量创建**：一次只能提交一个 URL，多个视频要循环调用
4. **任务列表不分页**：`GET /api/tasks` 返回全部，任务很多时响应会比较大
5. **日志接口没有 `tail` 参数**：返回完整日志，长任务时很大

这几项如果对自动化场景比较重要，可以提 issue。
