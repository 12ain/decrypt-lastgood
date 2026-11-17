# Changelog

格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号用
[语义化版本](https://semver.org/lang/zh-CN/)。

## [0.2.0] - 2026-11-17

桌面客户端。命令行保持兼容，参数、输出、退出码都没动。

### Added

- `gui/`：原生绘制的窗口客户端（MyGo `ui` 包，Metal / Direct3D 11 / OpenGL，无 webview）。
  三个页面分别对应默认命令、`download`、`decrypt`：获取凭据、仅下载、解密本地文件
- 窗口里的账号卡片：账号与密码一键复制，密码默认打码，点「显示」才明文
- 运行日志按步骤记录；勾上「记录 telnet 会话」等于命令行的 `-v`，原始收发也进日志
- 连不上 telnet 时直接给开启 Telnet 页面的链接，点了用系统浏览器打开
- `Enter` 提交表单，`Cmd+Enter` / `Ctrl+Enter` 随处运行；运行期间锁住表单，避免两次会话抢端口
- 把 `.enc` 拖到窗口任意位置，自动切到解密页并填好路径
- 应用图标：深色圆角方 + 钥匙孔，替换模板默认图标
- `pkg/dlg`：命令行与窗口共用的库层（连接、取文件、解密、提取账号、写文件、错误分类）
- `build-gui.sh`：用 `mygo build` 打包成 `.app`/`.dmg`、`.exe`/安装包、`.deb`/`.rpm`/`PKGBUILD`
- `docs/GUI.md`；`docs/images/` 里的界面图由视图测试生成，可重复产出
- 视图测试（`ui.NewTester`，不开窗口、不连光猫）：首次内容、完整获取、复制、连接失败提示、
  表单校验、三个页面、`-v` 记录、日志截断、忙时锁表单、Tab 输入、两个快捷键、拖放文件

### Changed

- 命令行改为调用 `pkg/dlg`，与窗口走同一条实现，避免两份逻辑漂移
- 连不上端口时的错误行现在以「连不上设备：」开头，后面跟原来的 `connect to ...` 原文；
  提示页的行为不变
- README 补桌面客户端、共用库层与两套测试的说明；`CONTRIBUTING.md` 补 `gui/` 的构建与约定
- 窗口版与命令行合成一个 Go module：`gui` 从独立 module 变成同一个 module 里的一个包，
  `go.mod` 统一到 MyGo 要求的 1.27.3，`go tool mygo` 在仓库任何位置可用
- 命令行的导入图里仍然没有第三方包，`dlg` 编出来的二进制与合并前同尺寸（3,161,138 字节）；
  CI 改为按导入图检查这一点，而不是检查模块依赖列表

### Notes

- 只编命令行也需要 Go 1.27.3 以上：工具链版本由 module 统一，这是合并的代价
- 运行中还不能取消，最长等到下载的 120 秒超时

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

[0.2.0]: https://github.com/12ain/decrypt-lastgood/releases/tag/v0.2.0
[0.1.0]: https://github.com/12ain/decrypt-lastgood/releases/tag/v0.1.0
