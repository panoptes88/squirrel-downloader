# 松鼠下载器 · Squirrel Downloader

自托管的 m3u8 / HLS 视频下载与管理工具，基于 **Go + Gin + Vue3 + Ant Design Vue**，
底层调用 [N_m3u8DL-RE](https://github.com/nilaoda/N_m3u8DL-RE) 完成下载与合流。

本仓库面向 **绿联 UGOS Pro 应用中心** 上架，产出 `ugcli` 可打包的 UPK 安装包。

> 合规提醒：请仅用于下载你拥有版权或已获得授权的内容。本应用不提供、不内置任何内容源。

---

## 功能

- 粘贴 m3u8 / mpd / mp4 链接即可创建下载任务，实时查看进度与日志
- 自动选择最佳视频 + 音频流并合流为 MP4，分片并行下载
- 下载方案：按域名保存常用配置（请求头、代理、解密密钥、自定义参数），新建任务自动匹配
- 文件管理：浏览、播放、下载、批量删除成品文件
- 单用户鉴权（`admin` + 安装时设定的密码），支持应用内改密
- 全部数据留在本机：SQLite 存任务，成品文件写入你选择的目录
- 界面支持中文（默认）与英文切换，语言偏好保存在当前浏览器中；下载日志与历史任务错误保留原文

## 目录结构

```
.
├── cmd/server/                 # Go 后端入口
├── internal/                   # 配置 / 处理器 / 中间件 / 模型 / 业务逻辑
├── web/                        # Vue3 前端（Vite 构建）
├── scripts/
│   ├── build-tools.sh          # 构建两个架构的 N_m3u8DL-RE / ffmpeg / mp4decrypt
│   ├── make_icon.py            # 生成 256x256 应用图标
│   └── extract_template_icon.py
├── dist/tools/{amd64,arm64}/   # 外部工具产物（构建中间产物，不入库）
├── Dockerfile                  # 多架构镜像（buildx）
├── docker-compose.yml          # 本地开发用
├── Makefile                    # 构建 / 打包入口
└── ugreen/                     # ★ 绿联 UGOS 应用工程（ugcli 的工作目录）
    ├── project.yaml
    ├── rootfs_common/
    │   ├── icon.png            # 256x256 应用图标
    │   └── docker-compose.yaml # 安装后实际运行的编排文件
    ├── rootfs_amd64/images/    # amd64 镜像 tar
    └── rootfs_arm64/images/    # arm64 镜像 tar
```

## 构建与打包

前置：Linux（Debian 12+ 推荐）、Docker（含 buildx）、Python3 + Pillow、约 5 GB 磁盘。

```bash
# 1) 生成图标（可选，仓库已带成品）
make icon

# 2) 构建两个架构的外部工具（首次约 15-30 分钟，之后走缓存）
make tools

# 3) 构建两个架构的镜像并导出为 tar 到 ugreen/rootfs_{arch}/images/
make images

# 4) 校验工程 + 打包 UPK（构建号 1 -> 版本 0.1.0.0001）
make check
make upk BUILD=1
```

一条龙：`make all`（等价于上面 1→4）。

产物：`ugreen/build_dir/pkgs/upk/`

```
amd64_nasync_com.panoptes88.squirrel_0.1.0.0001.upk   60 MB
arm64_nasync_com.panoptes88.squirrel_0.1.0.0001.upk   59 MB
```

### 构建状态（0.1.0.0001 已实测）

| 检查项 | 结果 |
| --- | --- |
| `ugcli check` | ✅ `✓ check passed` |
| `ugcli pack --build 1` | ✅ 两个架构均生成 UPK |
| amd64 镜像运行 | ✅ `x86_64`，`/api/health` 返回 ok |
| arm64 镜像运行（QEMU） | ✅ `aarch64`，三个外部工具均可执行 |
| 完整下载链路 | ✅ 建任务 → N_m3u8DL-RE 拉流 → ffmpeg 合流 → 输出可播放 MP4（含 h264 + aac 两轨） |
| 登录鉴权 | ✅ 正确密码 200、错误密码 401 |
| 外部工具版本 | N_m3u8DL-RE `0.5.1+c1f6db5…`、FFmpeg `7.1.1`、mp4decrypt `Bento4 1.6.0.0` |
| 镜像内许可文本 | ✅ `/app/THIRD_PARTY_LICENSES.md` + `/app/licenses/{MIT,LGPL-2.1,GPL-2.0}.txt` |

> 外部工具总共只有 46 MB（两个架构合计），因为 ffmpeg 是按实际能力裁剪自建的
> （3.7 MB，替代官方 137 MB 全量包），详见 `scripts/build-tools.sh`。

`ugcli` 不在仓库里，请从绿联开发者平台下载后放到 `.tools/ugcli`：

```bash
curl -sL -o .tools/ugcli \
  https://osswaf.ugnas.com/pro/ugcli/download/ugcli-v1.1.0.25-linux-amd64
chmod +x .tools/ugcli
```

> 必须在 **Linux** 上执行 `ugcli pack`，Windows 无法保留包内 Unix 文件权限。

## 本地开发

```bash
make dev          # 构建并启动，打开 http://127.0.0.1:21808
make dev-down
```

默认账号 `admin` / `admin123`（仅本地开发用；正式安装由 `ADMIN_PASSWORD` 参数决定）。

## 装机测试

1. 确认 NAS 固件版本 ≥ `1.13.0.0000`，并已安装 Docker 套件 ≥ `1.7.0.0000`。
2. 向绿联申请开发者授权，把 `ugdev.sig` 上传到管理员个人文件夹；
   固件 ≥ `1.16.0.0000` 时还需在「应用中心 > 设置 > 应用开发设置」点一次**授权**。
3. 应用中心 → 手动安装 → 选择与机型匹配的 UPK（NAS 只能装 `nasync` 包）。

## 上架资料清单

| 字段 | 状态 | 说明 |
| --- | --- | --- |
| `privacy_policy_link` | ✅ 正文已写好 | [docs/privacy.html](./docs/privacy.html)，含隐私政策正文 + 三清单。零外部依赖的静态页，托管到任意静态服务即可 |
| `license_agreement_link` | ✅ 已填 | 指向本仓库的 [LICENSE](./LICENSE) |
| `source_code_link` | ✅ 已填 | 本仓库地址 |
| `technical_support_link` | ✅ 已填 | Issue 区 |

> 隐私政策页面需托管后才生效。若面向中国大陆上架，建议用自己已备案的域名，
> 不要用 `github.io`（境内经常不可达）。换地址后只需改 `project.yaml` 并重新 `ugcli pack`。

如计划在中国大陆地区分发，除本仓库内容外还需自行准备：软件著作权登记证书、
ICP 许可/备案、应用备案、公安联网备案。具体要求以绿联云开发者平台的最新文档为准。

## 隐私

**本应用不收集任何个人信息，也不向任何第三方发送数据。** 全部数据（账号、任务、
下载方案、成品文件）都只保存在你自己的 NAS 上；开发者不运营任何云端服务。

详见 [隐私政策与三清单](./docs/privacy.html)。

## 许可

本项目**自己编写的代码**（Go 后端、Vue 前端、构建脚本、图标与文档）以
[MIT](./LICENSE) 许可发布：

```
Copyright (c) 2026 panoptes88
```

运行时会调用以下三个**独立分发**的第三方可执行程序，它们各自遵循自己的许可，
不受本项目 MIT 许可约束。版权声明、许可证全文与源码获取方式见
[THIRD_PARTY_LICENSES.md](./THIRD_PARTY_LICENSES.md) 与 [licenses/](./licenses/)：

| 组件 | 版本 | 许可证 |
| --- | --- | --- |
| N_m3u8DL-RE | v0.5.1-beta | MIT |
| FFmpeg（自建最小化） | 7.1.1 | LGPL-2.1-or-later |
| Bento4 `mp4decrypt` | v1.6.0-641 | GPL-2.0（双许可） |
| 应用图标 | — | 本项目原创，无第三方约束 |

> 三者以独立可执行文件形式打包、由本应用通过命令行调用（聚合分发），
> 因此不会把本项目的代码置于其许可之下。
