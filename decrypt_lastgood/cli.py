from __future__ import annotations

import argparse
import getpass
import os
from pathlib import Path

from .crypto import LastgoodDecryptError, decrypt_lastgood_xml
from .telnet_fetch import TelnetFetchError, TelnetTarget, fetch_remote_file

DEFAULT_REMOTE_PATH = "/config/work/lastgood.xml"


def main(argv: list[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)
    try:
        return args.func(args)
    except (LastgoodDecryptError, TelnetFetchError, OSError, ValueError) as exc:
        parser.exit(1, f"error: {exc}\n")


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="dlg",
        description="下载或解密光猫 lastgood.xml 配置备份。",
    )
    subparsers = parser.add_subparsers(dest="command", required=True)

    download = subparsers.add_parser("download", aliases=["dl"], help="下载加密配置文件")
    _add_telnet_args(download)
    download.add_argument("-o", "--output", type=Path, default=Path("lastgood.xml.enc"))
    download.set_defaults(func=cmd_download)

    decrypt = subparsers.add_parser("decrypt", aliases=["dec"], help="解密本地加密文件")
    decrypt.add_argument("input", type=Path)
    decrypt.add_argument("-o", "--output", type=Path, default=Path("lastgood.xml"))
    decrypt.set_defaults(func=cmd_decrypt)

    return parser


def cmd_download(args: argparse.Namespace) -> int:
    data = fetch_remote_file(_target_from_args(args), args.remote_path)
    args.output.write_bytes(data)
    print(f"文件已下载：{os.path.abspath(args.output)}")
    return 0


def cmd_decrypt(args: argparse.Namespace) -> int:
    xml_data = decrypt_lastgood_xml(args.input.read_bytes())
    args.output.write_bytes(xml_data)
    print(f"文件已解密：{os.path.abspath(args.output)}")
    return 0


def _add_telnet_args(parser: argparse.ArgumentParser) -> None:
    parser.add_argument("host", help="光猫 IP 地址")
    parser.add_argument("-u", "--username", required=True, help="telnet 用户名")
    parser.add_argument("-p", "--password", help="telnet 密码；省略后交互输入")
    parser.add_argument("-P", "--port", type=int, default=23, help="telnet 端口，默认 23")
    parser.add_argument(
        "-r",
        "--remote",
        dest="remote_path",
        default=DEFAULT_REMOTE_PATH,
        help=f"远端文件路径，默认 {DEFAULT_REMOTE_PATH}",
    )
    parser.add_argument("-t", "--timeout", type=float, default=8.0, help="连接超时秒数")


def _target_from_args(args: argparse.Namespace) -> TelnetTarget:
    password = args.password
    if password is None:
        password = getpass.getpass("Telnet password: ")
    return TelnetTarget(
        host=args.host,
        port=args.port,
        username=args.username,
        password=password,
        timeout=args.timeout,
    )


if __name__ == "__main__":
    raise SystemExit(main())
