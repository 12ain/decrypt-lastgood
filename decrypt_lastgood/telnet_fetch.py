import base64
import re
import telnetlib
import time
from dataclasses import dataclass


class TelnetFetchError(RuntimeError):
    pass


@dataclass(frozen=True)
class TelnetTarget:
    host: str
    username: str
    password: str
    port: int = 23
    timeout: float = 8.0


class TelnetSession:
    def __init__(self, target: TelnetTarget):
        self._target = target
        self._tn: telnetlib.Telnet | None = None
        self._counter = 0

    def __enter__(self) -> "TelnetSession":
        self._tn = telnetlib.Telnet(
            self._target.host,
            self._target.port,
            timeout=self._target.timeout,
        )
        time.sleep(0.8)
        banner = self._read_some(3.0)
        if b"login" in banner.lower() or b"username" in banner.lower():
            self._send(self._target.username)
            time.sleep(0.6)
            self._read_some(2.0)
        self._send(self._target.password)
        time.sleep(1.0)
        login_output = self._read_some(3.0)
        if b"incorrect" in login_output.lower() or b"failed" in login_output.lower():
            raise TelnetFetchError("telnet login failed")
        return self

    def __exit__(self, exc_type, exc, tb) -> None:
        if self._tn is not None:
            try:
                self._send("exit")
            finally:
                self._tn.close()

    def fetch_file(self, remote_path: str) -> bytes:
        base64_path = self.run("command -v base64", timeout=8.0).strip()
        if not base64_path:
            raise TelnetFetchError("remote device does not provide base64")

        # 设备侧只执行只读命令：先取大小，再用 base64 把二进制配置安全带回本机。
        size_text = self.run(f"wc -c {remote_path}", timeout=8.0)
        expected_size = _parse_wc_size(size_text)
        encoded = self.run(f"base64 {remote_path}", timeout=120.0)
        lines = [
            line.strip()
            for line in encoded.splitlines()
            if re.fullmatch(r"[A-Za-z0-9+/=]+", line.strip())
        ]
        if not lines:
            raise TelnetFetchError(f"no base64 output returned for {remote_path}")
        data = base64.b64decode("".join(lines))
        if len(data) != expected_size:
            raise TelnetFetchError(
                f"downloaded size mismatch: got {len(data)}, expected {expected_size}"
            )
        return data

    def run(self, command: str, timeout: float = 30.0) -> str:
        if self._tn is None:
            raise TelnetFetchError("telnet session is not connected")

        self._counter += 1
        start = f"__DLG_START_{self._counter}__"
        end = f"__DLG_END_{self._counter}__"
        # 用唯一标记包住命令输出，避免把登录提示符或回显误当成文件内容。
        self._send(f"echo {start}")
        self._send(command)
        self._send(f"echo {end}")

        buf = b""
        deadline = time.time() + timeout
        while time.time() < deadline and end.encode("ascii") not in buf:
            try:
                chunk = self._tn.read_very_eager()
            except EOFError as exc:
                raise TelnetFetchError("telnet connection closed") from exc
            if chunk:
                buf += chunk
            time.sleep(0.1)

        text = buf.decode("latin1", errors="replace")
        if start in text:
            text = text.split(start, 1)[1]
        if end in text:
            text = text.split(end, 1)[0]
        return _clean_telnet_output(text, command)

    def _send(self, line: str) -> None:
        if self._tn is None:
            raise TelnetFetchError("telnet session is not connected")
        self._tn.write(line.encode("ascii") + b"\n")

    def _read_some(self, timeout: float) -> bytes:
        if self._tn is None:
            raise TelnetFetchError("telnet session is not connected")
        end = time.time() + timeout
        out = b""
        while time.time() < end:
            chunk = self._tn.read_very_eager()
            if chunk:
                out += chunk
                end = time.time() + 0.45
            time.sleep(0.05)
        return out


def fetch_remote_file(target: TelnetTarget, remote_path: str) -> bytes:
    with TelnetSession(target) as session:
        return session.fetch_file(remote_path)


def _parse_wc_size(text: str) -> int:
    for token in text.split():
        if token.isdigit():
            return int(token)
    raise TelnetFetchError(f"could not parse remote file size from: {text!r}")


def _clean_telnet_output(text: str, command: str) -> str:
    cleaned: list[str] = []
    for line in text.splitlines():
        line = re.sub(r"\x1b\[[0-9;]*[A-Za-z]", "", line).strip()
        if not line:
            continue
        if "__DLG_START_" in line or "__DLG_END_" in line:
            continue
        if line == command:
            continue
        if line.startswith("echo __DLG_"):
            continue
        if line.endswith(" $") or line.startswith("~ $"):
            continue
        cleaned.append(line)
    return "\n".join(cleaned)
