#!/usr/bin/env python3
"""生成「松鼠下载器」的应用图标（基于 `assets/app-icon-source.png`）。

绿联的图标规范（developer.ugnas.com）：
  - 尺寸 256x256 px
  - 格式 PNG，文件大小 < 100 KB
  - 形状：正方形，套用「平滑圆角模板」
  - 背景：白色或浅色
  - 描边：0.5 px 粗、8% 黑

原图是「不透明白底 + 深色圆角卡片」。直接交上去的话，四角是白色而不是透明，
在 UGOS 桌面上会变成一块带白框的方形贴片，和系统里其它应用图标不一致。
所以这里做两件事：

  1. 套用平台模板的圆角轮廓（取自 ugcli 内置模板图标），把四角变透明，
     让深色卡片本身成为图标外形，与系统图标视觉统一。
  2. 沿轮廓补上规范要求的 0.5 px、8% 黑描边。0.5 px 无法直接绘制，
     改为在 4 倍画布上画 2 px 再降采样，等价且带抗锯齿。

依赖：pillow
用法：
    python3 scripts/make_icon.py

产出：
    ugreen/rootfs_common/icon.png        打包用的 256x256 图标
    web/public/favicon.png               站点图标
    web/src/assets/logo.png              应用内品牌 logo
    .assets/icon-preview-512.png         512 预览
    .assets/icon-variant-framed.png      备选：保留白色边框的版本
"""

from __future__ import annotations

import pathlib
import sys

from PIL import Image, ImageDraw, ImageFilter

ROOT = pathlib.Path(__file__).resolve().parent.parent
ASSETS = ROOT / ".assets"
SOURCE = ROOT / "assets" / "app-icon-source.png"

# 绿联官方「平滑圆角模板」（icon_template.svg，64x64 squircle 路径）栅格化后的 alpha 轮廓。
# 来源：上架规范附件 平滑圆角模板.zip，规范要求图标必须套用该模板。
TEMPLATE_PNG = ASSETS / "template-icon-official-256.png"

SIZE = 256
SUPERSAMPLE = 4
STROKE_RGBA = (0, 0, 0, 20)  # 约 8% 黑
STROKE_WIDTH_AT_SUPER = 2  # 4 倍画布上 2px -> 256 画布上 0.5px


def rounded_mask(size: int) -> Image.Image:
    """平滑圆角方形轮廓；优先复用 ugcli 模板图标的 alpha，保证与系统一致。"""
    if TEMPLATE_PNG.exists():
        tmpl = Image.open(TEMPLATE_PNG).convert("RGBA")
        return tmpl.split()[3].resize((size, size), Image.LANCZOS)
    # 退化路径：自绘圆角矩形（半径约为边长的 18.75%，与模板实测一致）
    big = size * 4
    mask = Image.new("L", (big, big), 0)
    ImageDraw.Draw(mask).rounded_rectangle(
        [0, 0, big - 1, big - 1], radius=int(big * 0.1875), fill=255
    )
    return mask.resize((size, size), Image.LANCZOS)


def optimize_png(
    img: Image.Image,
    path: pathlib.Path,
    hard_limit_kb: float = 100.0,
    prefer_below_kb: float = 60.0,
) -> None:
    """保存 PNG，并给 100 KB 的硬上限留出余量。

    先直接存；若超过 prefer_below_kb 就改成「只量化 RGB、保留原始 8 位 alpha」。
    这一点很关键：如果连 alpha 一起量化，圆角的抗锯齿会被打掉、变成硬边锯齿。
    """

    def save(im: Image.Image) -> int:
        im.save(path, "PNG", optimize=True)
        return path.stat().st_size

    size = save(img)
    if size / 1024 < prefer_below_kb:
        return

    alpha = img.split()[3]
    for colors in (256, 192, 128, 96):
        q = img.convert("RGB").quantize(colors=colors, method=Image.FASTOCTREE)
        q = q.convert("RGBA")
        q.putalpha(alpha)  # 保留原始抗锯齿 alpha
        size = save(q)
        if size / 1024 < hard_limit_kb:
            return

    print(
        f"  ⚠️ {path.name} 仍为 {size / 1024:.1f} KB，超过 {hard_limit_kb} KB 上限",
        file=sys.stderr,
    )


def build_icon(apply_rounded: bool = True) -> Image.Image:
    """按 4 倍超采样合成，再降回 256，保证描边和边缘抗锯齿。"""
    big = SIZE * SUPERSAMPLE
    src = Image.open(SOURCE).convert("RGBA").resize((big, big), Image.LANCZOS)

    if not apply_rounded:
        return src.resize((SIZE, SIZE), Image.LANCZOS)

    mask = rounded_mask(big)

    # 0.5px 的 8% 黑描边 = 「轮廓 减去 腐蚀后的轮廓」得到的内描边环
    eroded = mask.filter(ImageFilter.MinFilter(STROKE_WIDTH_AT_SUPER * 2 + 1))
    ring = Image.new("L", (big, big), 0)
    ring.paste(mask, (0, 0))
    # composite(a, b, m)：m=255 处取 a，m=0 处取 b
    ring = Image.composite(Image.new("L", (big, big), 0), ring, eroded)

    stroke = Image.new("RGBA", (big, big), STROKE_RGBA)
    # 只有原图边缘透出去的地方才需要描边；深色卡片上描黑几乎不可见，但满足规范
    out = Image.alpha_composite(
        src, Image.composite(stroke, Image.new("RGBA", (big, big), (0, 0, 0, 0)), ring)
    )
    out.putalpha(mask)
    return out.resize((SIZE, SIZE), Image.LANCZOS)


def main() -> None:
    if not SOURCE.exists():
        raise SystemExit(f"缺少图标源文件 {SOURCE.relative_to(ROOT)}")

    icon = build_icon(apply_rounded=True)
    assert icon.size == (SIZE, SIZE), icon.size

    targets = [
        ROOT / "ugreen" / "rootfs_common" / "icon.png",
        ROOT / "web" / "public" / "favicon.png",
        ROOT / "web" / "src" / "assets" / "logo.png",
    ]
    for path in targets:
        path.parent.mkdir(parents=True, exist_ok=True)
        optimize_png(icon, path)
        kb = path.stat().st_size / 1024
        print(f"wrote {path.relative_to(ROOT)} ({kb:.1f} KB) {'OK' if kb < 100 else '超限!'}")

    # 预览图不受大小限制，用无损放大方便肉眼检查
    icon.resize((512, 512), Image.LANCZOS).save(ASSETS / "icon-preview-512.png")
    print(f"wrote {(ASSETS / 'icon-preview-512.png').relative_to(ROOT)}")

    # 备选：保留原图白底与方形边界（字面最贴近「浅色背景」，但会多一圈白框）
    framed = build_icon(apply_rounded=False)
    framed.resize((512, 512), Image.LANCZOS).save(ASSETS / "icon-variant-framed.png")
    print(f"wrote {(ASSETS / 'icon-variant-framed.png').relative_to(ROOT)} (备选)")


if __name__ == "__main__":
    main()
