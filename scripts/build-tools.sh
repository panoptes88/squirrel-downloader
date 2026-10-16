#!/usr/bin/env bash
# shellcheck shell=bash
# =============================================================================
# scripts/build-tools.sh
#
# 作用：一条命令构建本 Web 应用依赖的三个外部命令行工具，并同时产出
#       linux/amd64 与 linux/arm64 两个 CPU 架构的二进制：
#
#   1) N_m3u8DL-RE —— .NET AOT 单文件程序，直接下载官方 Release 压缩包解包
#   2) ffmpeg      —— 自建“最小化”版本（LGPL，无 GPL 组件）：
#                     amd64 在构建平台原生编译；arm64 在 amd64 上交叉编译
#   3) mp4decrypt  —— Bento4 的 mp4decrypt：
#                     amd64 取官方 SDK 预编译包；arm64 由源码交叉编译
#
# 用法：
#   scripts/build-tools.sh [amd64|arm64|all]        # 默认 all
#
# 环境变量：
#   OUT_DIR=<dir>              输出目录，默认 <repo>/dist/tools
#   FORCE=1                    忽略缓存：重新下载、重新编译（默认 0）
#   JOBS=<n>                   编译并行度，默认容器内 nproc
#   DEBIAN_MIRROR=<url>        apt 主镜像，默认 http://deb.debian.org/debian
#   DEBIAN_SECURITY_MIRROR=<url> apt 安全镜像，默认 http://deb.debian.org/debian-security
#   FFMPEG_ARM64_MODE=cross|emulated
#                              arm64 ffmpeg 的构建方式，默认 cross（交叉编译）。
#                              若交叉编译在本机失败，可设 emulated 用 QEMU 模拟
#                              arm64 原生编译（更慢，但兼容性最好）。
#
# 产物：
#   <OUT_DIR>/amd64/{N_m3u8DL-RE,ffmpeg,mp4decrypt}
#   <OUT_DIR>/arm64/{N_m3u8DL-RE,ffmpeg,mp4decrypt}
#
# 设计约束（重要，改动前务必阅读）：
#   * 宿主机【不需要】任何 C/C++ 工具链：所有下载与编译都在 Docker 容器内完成，
#     编译容器基于 debian:trixie-slim（切勿在宿主机 apt-get install）。
#   * 构建工具链镜像按“Dockerfile 内容 + 镜像源”的哈希打 tag，重复执行时
#     自动复用，不会重复 apt-get install。
#   * 下载物缓存在 <repo>/.scratch/tools/cache/，编译目录在 <repo>/.scratch/tools/build/；
#     这两处以及 dist/tools 都不属于源码，可随时删除重建。
#   * 幂等：目标二进制已存在且架构正确时直接跳过；FORCE=1 可强制重建。
#   * 每一步结束都会校验产物的 ELF 架构，架构不对或文件缺失立即报错退出。
#
# ── 如何升级三个工具的版本（只需改本文件顶部的锚点常量）────────────────────
#   1) N_m3u8DL-RE：去 https://github.com/nilaoda/N_m3u8DL-RE/releases 找新 tag，
#      同时更新 RE_VERSION（如 v0.6.0-beta）与 RE_DATE（Release 资产名里的日期后缀，
#      形如 20251029）。注意上游 v0.6.0-beta 存在日志换行/tput 依赖问题，升级前请实测。
#   2) ffmpeg：更新 FFMPEG_VERSION，并保证 SHA 校验值（如启用校验）与
#      https://ffmpeg.org/releases/ffmpeg-<ver>.tar.xz 一致。
#      若新增功能依赖新的解封装/封装/滤镜，需要同步扩写 --enable-* 列表。
#   3) Bento4/mp4decrypt：更新 BENTO4_TAG 与 SDK 压缩包版本号；arm64 侧
#      目标平台名（Build/Targets/arm64-unknown-linux）一般跨版本稳定。
# =============================================================================

set -euo pipefail

# ---------------------------------------------------------------------------
# 0. 版本锚点（升级版本只改这里）
# ---------------------------------------------------------------------------
RE_VERSION="v0.5.1-beta"          # N_m3u8DL-RE 的 git tag
RE_DATE="20251029"                # 官方 Release 资产名中的日期后缀
RE_BASE_URL="https://github.com/nilaoda/N_m3u8DL-RE/releases/download"

FFMPEG_VERSION="7.1.1"            # ffmpeg 源码版本（官方 xz 包）
FFMPEG_URL="https://ffmpeg.org/releases/ffmpeg-${FFMPEG_VERSION}.tar.xz"

BENTO4_TAG="v1.6.0-641"           # Bento4 源码 tag（arm64 用）
BENTO4_SDK_VER="1-6-0-641"        # Bento4 SDK 版本号（amd64 预编译包用）
BENTO4_SDK_URL="https://www.bok.net/Bento4/binaries/Bento4-SDK-${BENTO4_SDK_VER}.x86_64-unknown-linux.zip"

# ---------------------------------------------------------------------------
# 1. 路径与参数解析
# ---------------------------------------------------------------------------
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd -- "$SCRIPT_DIR/.." && pwd)"

OUT_DIR="${OUT_DIR:-$ROOT_DIR/dist/tools}"
SCRATCH_DIR="$ROOT_DIR/.scratch/tools"     # 下载缓存 / 编译目录 / 日志
CACHE_DIR="$SCRATCH_DIR/cache"
BUILD_DIR="$SCRATCH_DIR/build"
LOG_DIR="$SCRATCH_DIR/logs"
BUILDER_CTX="$SCRATCH_DIR/builder"

FORCE="${FORCE:-0}"
FFMPEG_ARM64_MODE="${FFMPEG_ARM64_MODE:-cross}"
DEBIAN_MIRROR="${DEBIAN_MIRROR:-http://deb.debian.org/debian}"
# 注意：这里刻意使用 http://，因为 debian:trixie-slim 基础镜像内没有 ca-certificates，
# 首次 apt-get update 时无法校验 https 镜像。需要 https 时请先在该 Dockerfile 里
# 预置证书，或改成 http 镜像（apt 本身仍有 Release 签名校验）。

DEBIAN_SECURITY_MIRROR="${DEBIAN_SECURITY_MIRROR:-http://deb.debian.org/debian-security}"

ARCH_ARG="${1:-all}"
case "$ARCH_ARG" in
  amd64|arm64) ARCHES=("$ARCH_ARG") ;;
  all)         ARCHES=(amd64 arm64) ;;
  -h|--help)   sed -n '3,50p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
  *) echo "用法: $0 [amd64|arm64|all]" >&2; exit 2 ;;
esac

# OUT_DIR 必须是绝对路径：docker -v 只接受绝对路径
case "$OUT_DIR" in
  /*) : ;;
  *)  OUT_DIR="$(pwd)/$OUT_DIR" ;;
esac

# ---------------------------------------------------------------------------
# 2. 日志与错误处理
# ---------------------------------------------------------------------------
mkdir -p "$CACHE_DIR" "$BUILD_DIR" "$LOG_DIR" "$BUILDER_CTX" "$OUT_DIR"

log()  { printf '\033[1;34m[build-tools]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[build-tools][warn]\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31m[build-tools][error]\033[0m %s\n' "$*" >&2; exit 1; }

# ---------------------------------------------------------------------------
# 3. Docker 环境准备
# ---------------------------------------------------------------------------
command -v docker >/dev/null 2>&1 || die "未找到 docker 命令；本脚本要求所有构建都在 Docker 内完成。"

# 沙箱/受限环境下 $HOME/.docker/buildx 可能是只读挂载，buildx 会因写不了
# activity 临时文件而失败。这里做一次写探测，不可写就把 DOCKER_CONFIG 指到
# .scratch 下（只影响本次脚本，不修改用户全局配置）。
DOCKER_CFG="${DOCKER_CONFIG:-$HOME/.docker}"
if [ -d "$DOCKER_CFG" ] && ! { : > "$DOCKER_CFG/.build-tools-write-probe" ; } 2>/dev/null; then
  warn "$DOCKER_CFG 不可写，改用 $SCRATCH_DIR/docker-config 作为 DOCKER_CONFIG"
  export DOCKER_CONFIG="$SCRATCH_DIR/docker-config"
  mkdir -p "$DOCKER_CONFIG"
fi
rm -f "${DOCKER_CFG:-/nonexistent}/.build-tools-write-probe" 2>/dev/null || true

# 生成工具链 Dockerfile。所有编译期依赖都装在这里：
#   build-essential/nasm     → ffmpeg 原生编译
#   gcc/g++-aarch64-linux-gnu→ ffmpeg、Bento4 交叉编译 arm64（关键：见下方注释）
#   git/unzip/xz-utils/wget  → 拉取源码与预编译包
BUILDER_DOCKERFILE="$(cat <<'DOCKERFILE_EOF'
FROM debian:trixie-slim

ARG DEBIAN_MIRROR=http://deb.debian.org/debian
ARG DEBIAN_SECURITY_MIRROR=http://deb.debian.org/debian-security

ENV DEBIAN_FRONTEND=noninteractive

RUN set -eux; \
    if [ -f /etc/apt/sources.list.d/debian.sources ]; then \
      sed -i "s|http://deb.debian.org/debian-security|${DEBIAN_SECURITY_MIRROR}|g" /etc/apt/sources.list.d/debian.sources; \
      sed -i "s|http://deb.debian.org/debian|${DEBIAN_MIRROR}|g" /etc/apt/sources.list.d/debian.sources; \
    fi; \
    apt-get -o Acquire::Retries=10 -o Acquire::http::Timeout=60 update; \
    apt-get -o Acquire::Retries=10 -o Acquire::http::Timeout=60 install -y --no-install-recommends \
      ca-certificates wget curl git unzip zip xz-utils \
      build-essential nasm pkg-config cmake file \
      gcc-aarch64-linux-gnu g++-aarch64-linux-gnu binutils-aarch64-linux-gnu; \
    rm -rf /var/lib/apt/lists/*

WORKDIR /scratch
DOCKERFILE_EOF
)"

printf '%s\n' "$BUILDER_DOCKERFILE" > "$BUILDER_CTX/Dockerfile"

# 镜像 tag 内含内容哈希：Dockerfile 或镜像源变化时自动重建，否则直接复用。
BUILDER_KEY="$(printf '%s|%s|%s' "$BUILDER_DOCKERFILE" "$DEBIAN_MIRROR" "$DEBIAN_SECURITY_MIRROR" | sha256sum | cut -c1-12)"
BUILDER_IMAGE="squirrel-tools-builder:trixie-$BUILDER_KEY"
BUILDER_IMAGE_ARM64="squirrel-tools-builder-arm64:trixie-$BUILDER_KEY"

# build_builder_image <tag> [--platform linux/arm64]
build_builder_image() {
  local tag="$1"; shift
  if [ "$FORCE" != "1" ] && docker image inspect "$tag" >/dev/null 2>&1; then
    log "复用工具链镜像 $tag"
    return 0
  fi
  log "构建工具链镜像 $tag（首次约 1-3 分钟，之后按 tag 复用）..."
  docker build "$@" \
    --build-arg "DEBIAN_MIRROR=$DEBIAN_MIRROR" \
    --build-arg "DEBIAN_SECURITY_MIRROR=$DEBIAN_SECURITY_MIRROR" \
    -t "$tag" "$BUILDER_CTX" >"$LOG_DIR/builder-image.log" 2>&1 \
    || { tail -40 "$LOG_DIR/builder-image.log" >&2; die "工具链镜像构建失败，日志：$LOG_DIR/builder-image.log"; }
}

# run_step <step-name> <shell-script> [extra docker run 参数...]
# 在构建容器内执行一段脚本；日志落盘，失败时只回显尾部便于定位。
run_step() {
  local name="$1" script="$2"; shift 2
  log "▶ $name（完整日志：$LOG_DIR/$name.log）"
  if ! docker run --rm \
        -e "JOBS=${JOBS:-0}" \
        -e "ARCH=${STEP_ARCH:-}" \
        -e "CROSS=${STEP_CROSS:-0}" \
        -e "FFMPEG_VERSION=$FFMPEG_VERSION" \
        -e "FFMPEG_URL=$FFMPEG_URL" \
        -e "RE_VERSION=$RE_VERSION" -e "RE_DATE=$RE_DATE" -e "RE_BASE_URL=$RE_BASE_URL" \
        -e "BENTO4_TAG=$BENTO4_TAG" -e "BENTO4_SDK_URL=$BENTO4_SDK_URL" \
        -v "$SCRATCH_DIR:/scratch" \
        -v "$CACHE_DIR:/cache" \
        -v "$OUT_DIR:/out" \
        -w /scratch \
        "$@" \
        bash -euo pipefail -c "$script" >"$LOG_DIR/$name.log" 2>&1; then
    tail -40 "$LOG_DIR/$name.log" >&2
    die "步骤失败：$name（日志：$LOG_DIR/$name.log）"
  fi
}

# ---------------------------------------------------------------------------
# 4. 产物校验工具
# ---------------------------------------------------------------------------
# 直接读 ELF header 的 e_machine 字段（offset 18，2 字节），不依赖宿主机 file 命令。
elf_arch() {
  local f="$1" magic bytes
  [ -f "$f" ] || { echo "missing"; return; }
  magic=$(od -An -tx1 -N4 "$f" | tr -d ' \n')
  [ "$magic" = "7f454c46" ] || { echo "not-elf"; return; }
  bytes=$(od -An -tx1 -j18 -N2 "$f" | tr -d ' \n')
  case "$bytes" in
    3e00|003e) echo "amd64" ;;
    b700|00b7) echo "arm64" ;;
    *)         echo "unknown($bytes)" ;;
  esac
}

# 校验二进制存在且架构正确，否则“大声”失败
assert_binary() {
  local f="$1" want="$2" got
  [ -s "$f" ] || die "产物缺失或为空：$f"
  got=$(elf_arch "$f")
  [ "$got" = "$want" ] || die "产物架构错误：$f 期望 $want，实际 $got（file 输出：$(file -b "$f" 2>/dev/null || echo n/a)）"
}

human_size() { # bytes -> 人类可读
  awk -v b="$1" 'BEGIN{
    split("B KB MB GB",u," "); i=1;
    while (b>=1024 && i<4) { b/=1024; i++ }
    printf (i==1 ? "%d %s" : "%.2f %s"), b, u[i]
  }'
}

# 是否已有可用产物（幂等：存在 + 架构对 → 跳过）
is_cached() {
  local f="$1" want="$2"
  [ "$FORCE" != "1" ] || return 1
  [ -s "$f" ] || return 1
  [ "$(elf_arch "$f")" = "$want" ] || return 1
  return 0
}

# ===========================================================================
# 5. 工具一：N_m3u8DL-RE（官方 Release 二进制）
#    为什么不用编译：上游发布的是 .NET AOT 单文件可执行文件，官方已分别提供
#    linux-x64 与 linux-arm64 资产，直接下载即可，无需 .NET SDK。
# ===========================================================================
build_nm3u8dl_re() {
  local arch="$1" dest="$OUT_DIR/$arch/N_m3u8DL-RE"
  if is_cached "$dest" "$arch"; then log "跳过 N_m3u8DL-RE ($arch)：已存在"; return 0; fi

  local asset
  case "$arch" in
    amd64) asset="N_m3u8DL-RE_${RE_VERSION}_linux-x64_${RE_DATE}.tar.gz" ;;
    arm64) asset="N_m3u8DL-RE_${RE_VERSION}_linux-arm64_${RE_DATE}.tar.gz" ;;
  esac

  STEP_ARCH="$arch" STEP_CROSS=0
  run_step "nm3u8dl-re-$arch" '
    set -euo pipefail
    ASSET="'"$asset"'"
    URL="$RE_BASE_URL/$RE_VERSION/$ASSET"
    cd /cache
    if [ ! -s "$ASSET" ]; then
      echo "+ 下载 $URL"
      wget -q --tries=5 --timeout=120 -O "$ASSET.part" "$URL"
      mv "$ASSET.part" "$ASSET"
    else
      echo "+ 复用缓存 /cache/$ASSET"
    fi
    rm -rf /tmp/re && mkdir -p /tmp/re
    tar -xzf "$ASSET" -C /tmp/re
    SRC="$(find /tmp/re -type f -name N_m3u8DL-RE | head -n1)"
    [ -n "$SRC" ] || { echo "压缩包内未找到 N_m3u8DL-RE"; exit 1; }
    install -D -m 0755 "$SRC" "/out/'"$arch"'/N_m3u8DL-RE"
  ' "$BUILDER_IMAGE"
  assert_binary "$dest" "$arch"
}

# ===========================================================================
# 6. 工具二：ffmpeg（最小化自建，LGPL）
#
#    为什么 arm64 要交叉编译：本机是 linux/amd64，在 amd64 上编译 aarch64 目标
#    有两种办法——(a) 装 Debian 的 aarch64 交叉工具链直接产出 arm64 机器码；
#    (b) 用 QEMU 模拟 arm64 容器做“原生”编译。方案 (a) 快 5-10 倍，且不依赖
#    binfmt/QEMU，因此默认走交叉编译（FFMPEG_ARM64_MODE=cross）。
#
#    configure 参数集来自上游项目已实测验证的裁剪配方（由 N_m3u8DL-RE 源码
#    反推得出）：应用侧只做 -c copy 流拷贝，所以【不需要任何编码器】，只需
#    concat/file/pipe 协议 + 少量解封装/封装 + 比特流过滤器。
#    --disable-everything 后按需 --enable，产物约 3.9 MB
#    （若变成 60-140 MB 说明 --disable-everything 没生效）。
#    不含 --enable-gpl / --enable-nonfree，保持 LGPL。
# ===========================================================================
build_ffmpeg() {
  local arch="$1" dest="$OUT_DIR/$arch/ffmpeg"
  if is_cached "$dest" "$arch"; then log "跳过 ffmpeg ($arch)：已存在"; return 0; fi

  local mode="native" image="$BUILDER_IMAGE" platform_args=()
  local cross=0
  if [ "$arch" = "arm64" ]; then
    if [ "$FFMPEG_ARM64_MODE" = "emulated" ]; then
      mode="emulated(arm64 native under QEMU)"
      build_builder_image "$BUILDER_IMAGE_ARM64" --platform linux/arm64
      image="$BUILDER_IMAGE_ARM64"
      platform_args=(--platform linux/arm64)
      cross=0
    else
      mode="cross(aarch64-linux-gnu-)"
      cross=1
    fi
  fi
  log "ffmpeg $arch 构建方式：$mode"

  STEP_ARCH="$arch" STEP_CROSS="$cross"
  run_step "ffmpeg-$arch" '
    set -euo pipefail
    : "${JOBS:=0}"; [ "$JOBS" -gt 0 ] 2>/dev/null || JOBS="$(nproc)"
    SRC_VER="$FFMPEG_VERSION"
    SRC_TAR="ffmpeg-$SRC_VER.tar.xz"
    WORK="/build/ffmpeg-$SRC_VER-$ARCH"

    cd /cache
    if [ ! -s "$SRC_TAR" ]; then
      echo "+ 下载 $FFMPEG_URL"
      wget -q --tries=5 --timeout=180 -O "$SRC_TAR.part" "$FFMPEG_URL"
      mv "$SRC_TAR.part" "$SRC_TAR"
    else
      echo "+ 复用缓存 /cache/$SRC_TAR"
    fi

    if [ ! -f "$WORK/ffbuild/config.mak" ]; then
      rm -rf "$WORK"
      mkdir -p /build
      tar -xf "/cache/$SRC_TAR" -C /build
      mv "/build/ffmpeg-$SRC_VER" "$WORK"
    fi

    cd "$WORK"

    # 交叉编译专用参数：仅在 CROSS=1 时追加，保证 amd64 侧配方与已验证配方逐字一致
    CROSS_ARGS=()
    STRIP_BIN=strip
    if [ "$CROSS" = "1" ]; then
      CROSS_ARGS=(--enable-cross-compile --arch=aarch64 --target-os=linux
                  --cross-prefix=aarch64-linux-gnu- --cc=aarch64-linux-gnu-gcc)
      STRIP_BIN=aarch64-linux-gnu-strip
    fi

    if [ ! -f ffbuild/config.mak ]; then
      echo "+ ./configure ($ARCH)"
      ./configure \
        --prefix=/out --disable-everything \
        --disable-doc --disable-htmlpages --disable-manpages --disable-podpages --disable-txtpages \
        --disable-debug --disable-autodetect --disable-network \
        --disable-avdevice --disable-postproc --disable-swscale \
        --enable-ffmpeg --enable-static --disable-shared --enable-small \
        --enable-protocol=file,concat,pipe \
        --enable-demuxer=concat,mpegts,mov,matroska,flv,aac,ac3,eac3,h264,hevc,mpegvideo,mp3 \
        --enable-muxer=mp4,mov,matroska,flv,mpegts,adts,ac3,eac3,null \
        --enable-bsf=aac_adtstoasc,h264_mp4toannexb,hevc_mp4toannexb,extract_extradata \
        --enable-parser=aac,aac_latm,h264,hevc,ac3,mpegaudio,av1,vp9,opus,vorbis \
        --enable-decoder=aac,h264,hevc,ac3,eac3,mp3,opus,vorbis,flac \
        "${CROSS_ARGS[@]}"
    else
      echo "+ 复用已 configure 的构建目录 $WORK"
    fi

    echo "+ make -j$JOBS"
    make -j"$JOBS"

    "$STRIP_BIN" --strip-unneeded ffmpeg
    install -D -m 0755 ffmpeg "/out/$ARCH/ffmpeg"
  ' "${platform_args[@]}" "$image"
  assert_binary "$dest" "$arch"
}

# ===========================================================================
# 7. 工具三：mp4decrypt（Bento4）
#    amd64：官方 SDK zip 里自带预编译的 x86_64 mp4decrypt，直接用。
#    arm64：上游没有 arm64 Linux 预编译包，只能从源码交叉编译。
#           Bento4 自带 arm64-unknown-linux 目标，其 Local.mak 会 include
#           any-gnu-gcc/Local.mak，而后者使用 $(GCC_CROSS_PREFIX)g++/ar/strip，
#           所以只要传入 GCC_CROSS_PREFIX=aarch64-linux-gnu- 即可。
#
#           调用姿势很关键：必须 make -C Build/Targets/arm64-unknown-linux，
#           因为 Bootstrap.mak 用相对路径 include ../../Makefiles/Bootstrap.mak，
#           且随后 -C $(AP4_BUILD_CONFIG) -f ../Local.mak，都依赖当前工作目录为
#           该目标目录（从仓库根目录用 -f 指定 Makefile 会因相对路径解析失败）。
#           AP4_BUILD_CONFIG=Release 让编译带上 -O3（默认 Debug 只带 -g，无优化，
#           解密大文件会明显变慢）。
# ===========================================================================
build_mp4decrypt() {
  local arch="$1" dest="$OUT_DIR/$arch/mp4decrypt"
  if is_cached "$dest" "$arch"; then log "跳过 mp4decrypt ($arch)：已存在"; return 0; fi

  STEP_ARCH="$arch" STEP_CROSS=0
  if [ "$arch" = "amd64" ]; then
    run_step "mp4decrypt-$arch" '
      set -euo pipefail
      ZIP="$(basename "$BENTO4_SDK_URL")"
      cd /cache
      if [ ! -s "$ZIP" ]; then
        echo "+ 下载 $BENTO4_SDK_URL"
        wget -q --tries=5 --timeout=180 -O "$ZIP.part" "$BENTO4_SDK_URL"
        mv "$ZIP.part" "$ZIP"
      else
        echo "+ 复用缓存 /cache/$ZIP"
      fi
      rm -rf /tmp/b4 && mkdir -p /tmp/b4
      unzip -q -o -j "$ZIP" "*/bin/mp4decrypt" -d /tmp/b4
      strip --strip-unneeded /tmp/b4/mp4decrypt
      install -D -m 0755 /tmp/b4/mp4decrypt "/out/'"$arch"'/mp4decrypt"
    ' "$BUILDER_IMAGE"
  else
    run_step "mp4decrypt-$arch" '
      set -euo pipefail
      : "${JOBS:=0}"; [ "$JOBS" -gt 0 ] 2>/dev/null || JOBS="$(nproc)"
      SRC=/cache/Bento4
      if [ ! -d "$SRC/.git" ]; then
        rm -rf "$SRC"
        echo "+ 克隆 Bento4 $BENTO4_TAG"
        git clone --depth 1 --branch "$BENTO4_TAG" https://github.com/axiomatic-systems/Bento4.git "$SRC"
      else
        echo "+ 复用缓存 $SRC"
      fi
      # 每次强制编译前都从缓存克隆一份干净源码树，避免旧 .o 污染交叉编译结果
      WORK=/build/Bento4-arm64
      rm -rf "$WORK"
      mkdir -p /build
      cp -a "$SRC" "$WORK"
      cd "$WORK"
      echo "+ make -C Build/Targets/arm64-unknown-linux GCC_CROSS_PREFIX=aarch64-linux-gnu- AP4_BUILD_CONFIG=Release mp4decrypt"
      make -C Build/Targets/arm64-unknown-linux \
        GCC_CROSS_PREFIX=aarch64-linux-gnu- \
        AP4_BUILD_CONFIG=Release \
        -j"$JOBS" mp4decrypt
      BIN="$(find Build/Targets/arm64-unknown-linux/Release -maxdepth 1 -type f -name mp4decrypt | head -n1)"
      [ -n "$BIN" ] || { echo "未找到交叉编译产物 mp4decrypt"; exit 1; }
      aarch64-linux-gnu-strip --strip-unneeded "$BIN"
      install -D -m 0755 "$BIN" "/out/'"$arch"'/mp4decrypt"
    ' "$BUILDER_IMAGE"
  fi
  assert_binary "$dest" "$arch"
}

# ===========================================================================
# 8. 主流程
# ===========================================================================
main() {
  log "仓库根目录 : $ROOT_DIR"
  log "输出目录   : $OUT_DIR"
  log "架构       : ${ARCHES[*]}"
  log "FORCE      : $FORCE"

  build_builder_image "$BUILDER_IMAGE"

  for arch in "${ARCHES[@]}"; do
    mkdir -p "$OUT_DIR/$arch"
    log "──────── $arch ────────"
    build_nm3u8dl_re "$arch"
    build_ffmpeg      "$arch"
    build_mp4decrypt  "$arch"
  done

  # ---- 终检 + 体积汇总表 ----
  echo
  log "校验产物并汇总体积"
  printf '%-10s %-14s %-12s %-9s %s\n' "ARCH" "TOOL" "SIZE" "BYTES" "FILE"
  printf '%-10s %-14s %-12s %-9s %s\n' "----------" "--------------" "------------" "---------" "----"
  local total_all=0
  for arch in "${ARCHES[@]}"; do
    local total=0
    for tool in N_m3u8DL-RE ffmpeg mp4decrypt; do
      local f="$OUT_DIR/$arch/$tool" sz
      assert_binary "$f" "$arch"
      sz=$(stat -c %s "$f")
      total=$((total + sz)); total_all=$((total_all + sz))
      printf '%-10s %-14s %-12s %-9s %s\n' "$arch" "$tool" "$(human_size "$sz")" "$sz" "$(file -b "$f" 2>/dev/null || echo 'file 命令不可用')"
    done
    printf '%-10s %-14s %-12s %-9s %s\n' "$arch" "== TOTAL ==" "$(human_size "$total")" "$total" ""
  done
  if [ "${#ARCHES[@]}" -gt 1 ]; then
    printf '%-10s %-14s %-12s %-9s %s\n' "all" "== TOTAL ==" "$(human_size "$total_all")" "$total_all" ""
  fi
  echo
  log "完成。产物位于 $OUT_DIR/（架构：${ARCHES[*]}）"
}

main "$@"
