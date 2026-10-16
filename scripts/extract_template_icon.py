#!/usr/bin/env python3
"""从 ugcli 可执行文件中提取内置的模板应用图标（128x128 PNG）。

绿联的 ugcli 会把新建项目用的默认 icon.png 内嵌在二进制里。我们没有可交互的
TTY 来跑 `ugcli create`，所以直接扫描二进制里的 PNG 签名把模板取出来，
用作本项目图标的外轮廓参考。

用法：
    python3 scripts/extract_template_icon.py [ugcli 路径]
"""

from __future__ import annotations

import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
DEFAULT_UGCLI = ROOT / ".tools" / "ugcli"
OUT = ROOT / ".assets" / "template-icon-128.png"
PNG_SIG = b"\x89PNG\r\n\x1a\n"


def main() -> None:
    ugcli = pathlib.Path(sys.argv[1]) if len(sys.argv) > 1 else DEFAULT_UGCLI
    if not ugcli.exists():
        raise SystemExit(f"找不到 ugcli: {ugcli}")

    data = ugcli.read_bytes()
    best: tuple[int, bytes] | None = None
    for match in re.finditer(re.escape(PNG_SIG), data):
        start = match.start()
        end = data.find(b"IEND", start)
        if end == -1 or data[start + 12 : start + 16] != b"IHDR":
            continue
        blob = data[start : end + 8]
        width = int.from_bytes(blob[16:20], "big")
        height = int.from_bytes(blob[20:24], "big")
        if width == height and width >= 64:
            best = (width, blob)

    if best is None:
        raise SystemExit("未在 ugcli 中找到方形 PNG 图标")

    width, blob = best
    OUT.parent.mkdir(parents=True, exist_ok=True)
    OUT.write_bytes(blob)
    print(f"提取到 {width}x{width} 模板图标 -> {OUT.relative_to(ROOT)} ({len(blob)} bytes)")


if __name__ == "__main__":
    main()
