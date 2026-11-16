# Changelog

格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号用
[语义化版本](https://semver.org/lang/zh-CN/)。

## [Unreleased]

## [0.1.0] - 2026-11-16

首个 Go 版本，按上游 Python 实现的语义重写。

### Added

- `dlg download`：telnet 登录后把 `lastgood.xml` 以 base64 带回本地，按 `wc -c` 校验字节数
- `dlg decrypt`：解密容器，解包出配置 XML
- `--show-cred`：直接打印 `aucTeleAccountName` / `aucTeleAccountPassword`
- 自研 telnet 客户端，含 IAC 协商与跨包分片处理
- 自研 PBKDF2-HMAC-SHA256，不引入 `golang.org/x/crypto`
- 密码输入不回显（Windows `SetConsoleMode`，Unix `stty`）
- `build.sh`：交叉编译五个平台并输出 SHA-256
- `docs/FORMAT.md`、`docs/case-study-first-hop-packet-loss.md`

### Changed

- 仓库主体切换为 Go，模块路径 `github.com/12ain/decrypt-lastgood`

### Removed

- Python 实现及其依赖配置（`pyproject.toml`、`uv.lock`、`.python-version`）

[Unreleased]: https://github.com/12ain/decrypt-lastgood/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/12ain/decrypt-lastgood/releases/tag/v0.1.0
