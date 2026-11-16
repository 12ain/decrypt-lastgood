# Changelog

格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号用
[语义化版本](https://semver.org/lang/zh-CN/)。

## [0.1.0] - 2026-11-16

首个 Go 版本，按上游 Python 实现的语义重写。

### Added

- 默认命令 `dlg <光猫IP> -u <用户名> -p <密码>`：下载、解密、提取一次做完，直接输出
  超级管理员账号和密码（`download` / `decrypt` 两个子命令保留）
- `dlg download`：telnet 登录后把 `lastgood.xml` 以 base64 带回本地，按 `wc -c` 校验字节数
- `dlg decrypt`：解密容器，解包出配置 XML；`--show-cred` 直接打印账号密码
- `--show-cred` 兼容元素与 `<Value Name="aucTeleAccountName" Value="..."/>` 两种配置写法
- 自研 telnet 客户端：IAC 选项协商、跨包分片、设备回显与提示符粘连的容错
- 登录后先验证 shell 可用；base64 能力探测改为实跑 `echo aGk= | base64 -d`，支持 `busybox base64` 兜底
- `-v` 输出完整 telnet 会话记录（含 IAC 协商后的原始字节），便于排查固件差异
- 连不上 23 端口时打印开启 Telnet 的页面地址（`http://<光猫IP>/webcmcc/telnet_page.html`）
- 自研 PBKDF2-HMAC-SHA256，不引入 `golang.org/x/crypto`
- 密码输入不回显（Windows `SetConsoleMode`，Unix `stty`）
- `build.sh`：交叉编译五个平台并输出 SHA-256
- `docs/FORMAT.md`、`docs/case-study-first-hop-packet-loss.md`

### Fixed

- 命令输出的结束标记改成整行匹配：设备回显的 `echo __DLG_END_n__` 里也含标记文本，
  原来按子串匹配会在拿到输出前就结束读取，导致误报 `remote device does not provide base64`
- `Run` 超时不再静默返回半截输出，改为报错并带上命令名
- 提示符与输出粘在同一行时（如 `# 123456 /config/work/lastgood.xml`）不再丢数据；
  `base64 -d` 解码出的内容不带换行时（`hi~ $ `），也不会把 `hi` 连同提示符一起吃掉

### Changed

- 仓库主体切换为 Go，模块路径 `github.com/12ain/decrypt-lastgood`

### Removed

- Python 实现及其依赖配置（`pyproject.toml`、`uv.lock`、`.python-version`）

[0.1.0]: https://github.com/12ain/decrypt-lastgood/releases/tag/v0.1.0
