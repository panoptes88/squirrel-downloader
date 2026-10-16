# 松鼠下载器 —— 构建与打包入口
#
# 常用流程：
#   make icon          # 生成应用图标（需要 python3 + pillow）
#   make tools         # 构建 amd64/arm64 三个外部工具（较慢，仅需一次）
#   make images        # 构建两个架构的镜像并导出为 tar
#   make upk           # 校验并打包成 UPK
#   make all           # 上面全跑一遍（tools 已存在时会跳过）

SHELL := /bin/bash
.DEFAULT_GOAL := help

VERSION      ?= 0.1.0
BUILD        ?= 1
# 不要带 docker.io/ 前缀：docker save 会把它规范化掉，
# 而 ugcli 按字符串比对 compose 的 image 与 tar 里的 RepoTags。
REGISTRY     ?= panoptes88
IMAGE_NAME   ?= squirrel-downloader
IMAGE        := $(REGISTRY)/$(IMAGE_NAME)
TAG          := $(VERSION)
PLATFORMS    := amd64 arm64

UGCLI        ?= $(CURDIR)/.tools/ugcli
UGREEN_DIR   := ugreen
DIST_TOOLS   := dist/tools

# buildx 默认会写入 provenance/SBOM 证明清单，会让 docker save 的 tar 里
# 多出额外 manifest，而绿联要求「每个 tar 只能有一个 tag」，所以关掉。
BUILDX_FLAGS := --provenance=false --sbom=false --load

.PHONY: help
help:
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
	  | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

.PHONY: icon
icon: ## 生成 256x256 应用图标与前端 logo
	python3 scripts/extract_template_icon.py
	python3 scripts/make_icon.py

.PHONY: tools
tools: ## 构建 amd64/arm64 的 N_m3u8DL-RE / ffmpeg / mp4decrypt
	bash scripts/build-tools.sh all

.PHONY: tools-amd64
tools-amd64: ## 只构建 amd64 的外部工具
	bash scripts/build-tools.sh amd64

.PHONY: tools-arm64
tools-arm64: ## 只构建 arm64 的外部工具
	bash scripts/build-tools.sh arm64

.PHONY: image-amd64
image-amd64: $(DIST_TOOLS)/amd64 ## 构建 amd64 镜像并导出 tar
	docker buildx build --platform linux/amd64 $(BUILDX_FLAGS) \
	  -t $(IMAGE):$(TAG) .
	docker save -o $(UGREEN_DIR)/rootfs_amd64/images/$(IMAGE_NAME)-$(VERSION)-amd64.tar $(IMAGE):$(TAG)
	@ls -lh $(UGREEN_DIR)/rootfs_amd64/images/*.tar

.PHONY: image-arm64
image-arm64: $(DIST_TOOLS)/arm64 ## 构建 arm64 镜像并导出 tar
	docker buildx build --platform linux/arm64 $(BUILDX_FLAGS) \
	  -t $(IMAGE):$(TAG) .
	docker save -o $(UGREEN_DIR)/rootfs_arm64/images/$(IMAGE_NAME)-$(VERSION)-arm64.tar $(IMAGE):$(TAG)
	@ls -lh $(UGREEN_DIR)/rootfs_arm64/images/*.tar

# 两个架构共用同一个 RepoTag（compose 里只有一份 image 声明），
# 所以必须「构建一个架构就立刻导出」，再用同一个 tag 构建下一个架构。
# 否则后构建的架构会覆盖本地同名镜像，导出两次得到同一份东西。
.PHONY: images
images: image-amd64 image-arm64 ## 构建双架构镜像并导出 tar 到 ugreen/
	@echo ">> 两个架构的镜像 tar 已就绪"

.PHONY: check
check: ## 用 ugcli 校验工程配置
	cd $(UGREEN_DIR) && $(UGCLI) check

.PHONY: upk
upk: ## 打包成 UPK（默认 build 1）
	cd $(UGREEN_DIR) && $(UGCLI) pack --build $(BUILD)

.PHONY: all
all: icon tools images check upk ## 一条龙：图标 -> 工具 -> 镜像 -> 校验 -> 打包

.PHONY: dev
dev: ## 本地构建并运行（开发用，直接用仓库 Dockerfile）
	docker compose up -d --build
	@echo "打开 http://127.0.0.1:21808"

.PHONY: dev-down
dev-down: ## 停止本地开发容器
	docker compose down

.PHONY: check-ui
check-ui: ## 用真实浏览器量侧边栏对齐（需 playwright，见脚本头部说明）
	@echo "需要先起一个容器并设置 BASE_URL / APP_PASSWORD"
	node scripts/check-sidebar-alignment.js

.PHONY: clean
clean: ## 清理构建产物（保留 dist/tools 缓存）
	rm -rf $(UGREEN_DIR)/build_dir
	rm -f $(UGREEN_DIR)/rootfs_amd64/images/*.tar $(UGREEN_DIR)/rootfs_arm64/images/*.tar

.PHONY: distclean
distclean: clean ## 连 dist/tools 缓存一起清理
	rm -rf dist

# 依赖：外部工具就绪后才能构建镜像
$(DIST_TOOLS)/amd64 $(DIST_TOOLS)/arm64:
	bash scripts/build-tools.sh $(notdir $@)
