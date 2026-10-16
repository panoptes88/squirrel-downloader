# 第三方组件与开源许可 · Third-Party Notices

本应用（松鼠下载器 / Squirrel Downloader）**打包分发**了下列第三方程序，
并**链接**了若干开源库。按各许可证要求，此处保留版权声明、许可证标识与源码获取方式。

> **本项目自己的代码以 [MIT](./LICENSE) 许可发布**（Copyright (c) 2026 panoptes88）。
> 本文件列的是**第三方**组件，它们各自遵循自己的许可，不受 MIT 约束。
>
> 绿联审核要求（《应用程序审核规则》§四.3）：使用开源代码或组件的，须保留版权声明与
> 许可证文本、履行署名义务，并审慎评估传染性条款的影响。

---

## 一、随镜像分发的独立可执行程序

这三个程序以**独立可执行文件**形式随镜像分发，由本应用通过命令行调用。
它们不与本应用的代码静态链接，属于**聚合分发（mere aggregation）**，
因此不会将本应用的代码置于其许可证之下。

### 1. N_m3u8DL-RE

| 项目 | 内容 |
| --- | --- |
| 版本 | `v0.5.1-beta` |
| 许可证 | **MIT** |
| 版权 | Copyright (c) 2022 nilaoda |
| 源码 | <https://github.com/nilaoda/N_m3u8DL-RE> |
| 许可证全文 | [`licenses/MIT.txt`](./licenses/MIT.txt) |
| 修改情况 | **未修改**，直接使用官方发布的 Linux 二进制 |

### 2. FFmpeg（自建最小化构建）

| 项目 | 内容 |
| --- | --- |
| 版本 | `7.1.1`（源码来自 <https://ffmpeg.org/releases/ffmpeg-7.1.1.tar.xz>） |
| 许可证 | **LGPL-2.1-or-later** |
| 版权 | Copyright (c) 2000-2025 the FFmpeg developers |
| 源码 | <https://ffmpeg.org/download.html> |
| 许可证全文 | [`licenses/LGPL-2.1.txt`](./licenses/LGPL-2.1.txt) |
| 修改情况 | **仅裁剪编译选项，未修改源码**。未启用 `--enable-gpl` / `--enable-nonfree`，因此保持 LGPL，未引入 GPL 组件 |

本应用只使用 ffmpeg 做流拷贝（`-c copy`）封装，不调用任何编码器。
完整的 `./configure` 参数见 [`scripts/build-tools.sh`](./scripts/build-tools.sh)，
任何人可用同一份参数从上游源码重建出等价二进制——这满足 LGPL 对「可重新链接」的要求。

### 3. Bento4 `mp4decrypt`

| 项目 | 内容 |
| --- | --- |
| 版本 | `v1.6.0-641` |
| 许可证 | **GPL-2.0**（Bento4 采用 GPL / 商业 双许可） |
| 版权 | Copyright (c) 2002-2015 Axiomatic Systems, LLC |
| 源码 | <https://github.com/axiomatic-systems/Bento4>（tag `v1.6.0-641`） |
| 许可证全文 | [`licenses/GPL-2.0.txt`](./licenses/GPL-2.0.txt) |
| 修改情况 | **未修改源码**；arm64 版本由官方源码交叉编译得到（官方只发布 x86_64 二进制） |

⚠️ **GPL-2.0 具有传染性**。本应用通过命令行调用该独立程序、不与其链接，
故本应用自身代码不受 GPL 约束；但本程序本身的源码与许可证必须按上述方式提供。

**替代方案**（如希望彻底移除 GPL 依赖）：向 Axiomatic Systems 购买 Bento4 商业许可，
或在构建时排除 `mp4decrypt`（代价是失去加密流解密能力）。

---

## 二、图标与品牌素材

### 应用图标

| 项目 | 内容 |
| --- | --- |
| 文件 | `assets/app-icon-source.png`（源图）→ `ugreen/rootfs_common/icon.png` |
| 来源 | **本项目原创美术**，不包含任何第三方字形或素材 |
| 许可证 | 随本项目一起授权，无第三方署名或 ShareAlike 义务 |
| 加工方式 | 由 `scripts/make_icon.py` 套用平台圆角模板轮廓、补 0.5px 8% 黑描边，并做 PNG 体积优化 |

> 说明：早期版本曾用 OpenMoji 的 🐿️ 字形（CC BY-SA 4.0）拼图标，当时图标受
> ShareAlike 约束。现已替换为原创美术，该约束**已完全解除**，
> 商业品牌标识中不再残留任何 ShareAlike 素材。

---

## 三、Go 后端依赖（直接依赖）

均为宽松许可证（MIT / BSD-3-Clause），义务为保留版权声明与许可证文本。

| 模块 | 版本 | 许可证 |
| --- | --- | --- |
| github.com/gin-gonic/gin | v1.9.1 | MIT |
| github.com/gin-contrib/cors | v1.6.0 | MIT |
| github.com/google/uuid | v1.6.0 | BSD-3-Clause |
| golang.org/x/crypto | v0.53.0 | BSD-3-Clause |
| golang.org/x/net | v0.56.0 | BSD-3-Clause |
| gorm.io/gorm | v1.25.9 | MIT |
| gorm.io/driver/sqlite | v1.5.4 | MIT |

间接依赖（共 49 个模块版本，详见 `go.sum`）同样为 MIT / BSD / Apache-2.0 类宽松许可，
主要包括 `mattn/go-sqlite3`(MIT)、`bytedance/sonic`(Apache-2.0)、
`go-playground/validator`(MIT)、`pelletier/go-toml`(MIT)、
`golang.org/x/sys` / `golang.org/x/text`(BSD-3-Clause) 等。

## 四、Web 前端依赖（直接依赖）

| 包 | 版本 | 许可证 |
| --- | --- | --- |
| vue | ^3.4.0 | MIT |
| vue-router | ^4.2.5 | MIT |
| pinia | ^2.1.7 | MIT |
| ant-design-vue | ^4.0.0 | MIT |
| @ant-design/icons-vue | ^7.0.0 | MIT |
| axios | ^1.6.0 | MIT |
| dayjs | ^1.11.10 | MIT |
| xgplayer | ^3.0.23 | MIT |
| vite（构建期） | ^5.0.0 | MIT |
| less（构建期） | ^4.2.0 | Apache-2.0 |
| unplugin-vue-components（构建期） | ^32.1.0 | MIT |

前端构建产物由上述 MIT/Apache-2.0 组件打包而成，相关版权声明随包保留。

---

## 五、源码获取方式

- 本应用自身源码：见 `project.yaml` 的 `source_code_link`
- N_m3u8DL-RE：<https://github.com/nilaoda/N_m3u8DL-RE>
- FFmpeg：<https://ffmpeg.org/download.html>（构建参数见 `scripts/build-tools.sh`）
- Bento4：<https://github.com/axiomatic-systems/Bento4>
- OpenMoji：<https://github.com/hfg-gmuend/openmoji>

如对以上任何一项有疑问，请通过 `project.yaml` 中 `technical_support_link` 指向的渠道联系。

---

*最后更新：随 0.1.0 版本一起发布。*
