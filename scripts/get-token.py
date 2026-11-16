#!/usr/bin/env python3
"""获取松鼠下载器的 access_token。

服务端从 1.0.0.0018 起移除了明文登录接口（合规要求：HTTP 下密码不得明文传输），
登录必须走「取公钥 → RSA-OAEP + AES-GCM 加密 → 提交密文」的流程。
这个脚本把加密细节封装起来，调用方只需提供地址和密码。

依赖只有一个：cryptography（预编译 wheel，安装通常 30 秒内完成）

    pip install cryptography

用法：

    # 直接输出 token
    ./get-token.py --base http://192.168.1.50:8080 \\
                   --username admin --password '你的密码'

    # 用环境变量传密码，避免出现在命令历史里
    SQ_PASSWORD='你的密码' ./get-token.py --base http://192.168.1.50:8080

    # 输出成可直接喂给 curl 的形式
    TOKEN=$(./get-token.py --base http://192.168.1.50:8080)
    curl -H "Authorization: Bearer $TOKEN" http://192.168.1.50:8080/api/tasks

    # 也可以直接拿 cookie
    ./get-token.py --base http://192.168.1.50:8080 --format cookie

退出码：
    0 成功
    1 服务端拒绝（密码错误等）
    2 网络或加密失败
"""

from __future__ import annotations

import argparse
import base64
import json
import os
import sys
import urllib.error
import urllib.request

try:
    from cryptography.hazmat.primitives import hashes, serialization
    from cryptography.hazmat.primitives.asymmetric import padding
    from cryptography.hazmat.primitives.ciphers.aead import AESGCM
except ImportError:  # pragma: no cover
    sys.stderr.write(
        "缺少依赖 cryptography。安装方式：\n"
        "    pip install cryptography\n"
        "（预编译 wheel，通常 30 秒内装好，不需要编译器）\n"
    )
    sys.exit(2)


def build_encrypted_body(base: str, username: str, password: str) -> dict:
    """按服务端要求生成密文登录体。"""
    with urllib.request.urlopen(f"{base}/api/auth/encryption-key", timeout=10) as resp:
        key = json.loads(resp.read())

    public_key = serialization.load_pem_public_key(key["public_key"].encode())

    # 1) 随机 AES-256 会话密钥，用 RSA-OAEP(SHA-256) 包裹
    aes_key = AESGCM.generate_key(bit_length=256)
    encrypted_key = public_key.encrypt(
        aes_key,
        padding.OAEP(
            mgf=padding.MGF1(hashes.SHA256()),
            algorithm=hashes.SHA256(),
            label=None,
        ),
    )

    # 2) AES-256-GCM 加密凭据，challenge 一并放进明文（服务端消费后立即作废）
    iv = os.urandom(12)
    plaintext = json.dumps(
        {"username": username, "password": password, "challenge": key["challenge"]}
    ).encode("utf-8")
    # AESGCM.encrypt 的返回值已包含认证标签，正好对应 Go 的 cipher.GCM.Open
    ciphertext = AESGCM(aes_key).encrypt(iv, plaintext, None)

    return {
        "key_id": key["key_id"],
        "algorithm": key["algorithm"],
        "encrypted_key": base64.b64encode(encrypted_key).decode(),
        "iv": base64.b64encode(iv).decode(),
        "ciphertext": base64.b64encode(ciphertext).decode(),
    }


def login(base: str, username: str, password: str) -> tuple[str, str]:
    """返回 (access_token, auth_token_cookie)。"""
    body = build_encrypted_body(base, username, password)
    req = urllib.request.Request(
        f"{base}/api/auth/login-encrypted",
        data=json.dumps(body).encode(),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=15) as resp:
            data = json.loads(resp.read())
            set_cookie = resp.headers.get("Set-Cookie") or ""
    except urllib.error.HTTPError as e:
        detail = e.read().decode("utf-8", "replace")
        try:
            detail = json.loads(detail).get("error", detail)
        except Exception:
            pass
        sys.stderr.write(f"登录失败（HTTP {e.code}）：{detail}\n")
        sys.exit(1)

    token = data.get("access_token", "")
    cookie = ""
    for part in set_cookie.split(";"):
        part = part.strip()
        if part.startswith("auth_token="):
            cookie = part
            break
    return token, cookie


def main() -> int:
    parser = argparse.ArgumentParser(
        description="获取松鼠下载器的 access_token（密文登录）",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__.split("用法：")[1] if "用法：" in __doc__ else None,
    )
    parser.add_argument("--base", default=os.environ.get("SQ_BASE", "http://127.0.0.1:8080"),
                        help="NAS 地址，默认 $SQ_BASE 或 http://127.0.0.1:8080")
    parser.add_argument("--username", default=os.environ.get("SQ_USERNAME", "admin"),
                        help="用户名，默认 $SQ_USERNAME 或 admin")
    parser.add_argument("--password", default=os.environ.get("SQ_PASSWORD"),
                        help="密码，默认读 $SQ_PASSWORD")
    parser.add_argument("--format", choices=["token", "cookie", "json"], default="token",
                        help="输出格式：token（默认）/ cookie / json")
    args = parser.parse_args()

    if not args.password:
        sys.stderr.write("缺少密码。用 --password 或环境变量 SQ_PASSWORD 提供。\n")
        return 2

    base = args.base.rstrip("/")
    token, cookie = login(base, args.username, args.password)

    if args.format == "cookie":
        print(cookie)
    elif args.format == "json":
        print(json.dumps({"access_token": token, "cookie": cookie}, ensure_ascii=False))
    else:
        print(token)
    return 0


if __name__ == "__main__":
    sys.exit(main())
