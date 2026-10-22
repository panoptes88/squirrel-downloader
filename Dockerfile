# syntax=docker/dockerfile:1.7
#
# 松鼠下载器 —— 多架构镜像构建
#
# 该镜像把三样东西打包在一起：
#   1. Go 后端（Gin + SQLite，CGO 依赖目标平台 gcc）
#   2. Vue3 前端（Vite 构建产物，由后端以静态文件方式提供）
#   3. 三个外部命令行工具：N_m3u8DL-RE / ffmpeg / mp4decrypt
#
# 外部工具不在这里现场编译，而是由 scripts/build-tools.sh 预先产出到
# dist/tools/<arch>/。这样做的原因：
#   - arm64 的 ffmpeg 与 mp4decrypt 需要交叉编译（或 QEMU 模拟），
#     把这段慢且易失败的过程与镜像构建解耦，可单独重试与缓存；
#   - 镜像构建只需 COPY，产物可复现、可校验。
#
# 标准构建流程见 Makefile：先 `make tools`，再 `make image-*`。

############################
# 1. 前端构建
#    产物与架构无关，固定在构建平台上执行，避免 arm64 重复构建
############################
FROM --platform=$BUILDPLATFORM node:24-alpine AS web
WORKDIR /src

# 国内网络加速
RUN npm config set registry https://repo.huaweicloud.com/repository/npm/

COPY web/package.json web/package-lock.json ./
RUN npm ci

COPY web/ ./
RUN npm run build

############################
# 2. 后端构建
#    CGO（mattn/go-sqlite3）需要目标平台的 gcc：
#    amd64 为本机构建；arm64 由 buildx 通过 QEMU 以 arm64 容器运行后原生编译
############################
FROM golang:1.26 AS server
ARG TARGETOS=linux
ARG TARGETARCH

WORKDIR /src
ENV GOPROXY=https://goproxy.cn,direct
ENV CGO_ENABLED=1

# 依赖单独成层，改代码不会触发重新下载模块
COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ ./cmd
COPY internal/ ./internal
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

############################
# 3. 运行时镜像
############################
FROM debian:trixie-slim
ARG TARGETARCH

# N_m3u8DL-RE 是 .NET AOT 自包含程序，需要 libicu / libstdc++ / libgcc
#
# 可选：覆盖 Debian 软件源。官方源 deb.debian.org 在国内经常只有几 KB/s，
# 会让这一层卡几分钟甚至直接失败。国内构建建议加上：
#   docker buildx build \
#     --build-arg DEBIAN_MIRROR=http://mirrors.ustc.edu.cn/debian ...
#
# ⚠️ 这里必须用 http:// 而不是 https://：debian:trixie-slim 基础镜像内**没有
#    ca-certificates**，首次 apt-get update 时无法校验 HTTPS 镜像，会表现为
#    「找不到 ca-certificates / 找不到包」这类迷惑性报错。
#    走 http 并不降低安全性——apt 仍会用镜像内置的 debian-archive-keyring
#    校验 Release 签名，只是传输层没有 TLS。
ARG DEBIAN_MIRROR=
RUN set -eux; \
    if [ -n "$DEBIAN_MIRROR" ]; then \
      if [ -f /etc/apt/sources.list.d/debian.sources ]; then \
        sed -i "s|http://deb.debian.org/debian|${DEBIAN_MIRROR}|g" /etc/apt/sources.list.d/debian.sources; \
      fi; \
      if [ -f /etc/apt/sources.list ]; then \
        sed -i "s|http://deb.debian.org/debian|${DEBIAN_MIRROR}|g" /etc/apt/sources.list; \
      fi; \
    fi; \
    apt-get -o Acquire::Retries=5 update; \
    apt-get -o Acquire::Retries=5 install -y --no-install-recommends \
      ca-certificates \
      libgcc-s1 \
      libstdc++6 \
      libicu76 \
      tzdata \
      bash; \
    rm -rf /var/lib/apt/lists/*

# 时区在运行期由 UGOS 注入的 TZ 覆盖（compose 传入 ${TZ}）
ENV TZ=Asia/Shanghai
RUN ln -snf /usr/share/zoneinfo/$TZ /etc/localtime && echo $TZ > /etc/timezone

WORKDIR /app

# 数据目录：/app/db 存 SQLite，/app/downloads 存成品，均由 compose 挂载持久化
RUN mkdir -p /app/bin /app/db /app/downloads /app/Logs

# 外部工具（按架构由 scripts/build-tools.sh 预先生成）
COPY dist/tools/${TARGETARCH}/N_m3u8DL-RE /app/bin/N_m3u8DL-RE
COPY dist/tools/${TARGETARCH}/ffmpeg /app/bin/ffmpeg
COPY dist/tools/${TARGETARCH}/mp4decrypt /app/bin/mp4decrypt
RUN chmod 0755 /app/bin/N_m3u8DL-RE /app/bin/ffmpeg /app/bin/mp4decrypt

# 后端与前端
COPY --from=server /out/server ./server
COPY --from=web /src/dist ./web/dist

# 开源合规：把本项目的许可与第三方版权声明、许可证文本一起随镜像分发。
# 绿联审核规则 §四.3 要求「保留版权声明与许可证文本」。
COPY LICENSE /app/LICENSE
COPY THIRD_PARTY_LICENSES.md /app/THIRD_PARTY_LICENSES.md
COPY licenses/ /app/licenses/

# 默认值；实际由 compose 的 environment 覆盖
ENV PORT=8080 \
    DOWNLOAD_DIR=/app/downloads \
    BIN_DIR=/app/bin \
    DB_PATH=/app/db/data.db \
    LANG=C.UTF-8

EXPOSE 8080

# 健康检查打 /api/health（无需鉴权），并用响应体排除 SPA 兜底路由的假 200
HEALTHCHECK --interval=30s --timeout=10s --start-period=15s --retries=3 \
    CMD bash -c "exec 3<>/dev/tcp/127.0.0.1/8080 && printf 'GET /api/health HTTP/1.0\r\nHost: localhost\r\n\r\n' >&3 && grep -q '\"status\":\"ok\"' <&3"

CMD ["./server"]
