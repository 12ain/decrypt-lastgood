# decrypt-lastgood

通过 telnet 从九联 `UNG953H1-S` 光猫取回 `lastgood.xml`，在本地解密，拿到超级管理员
账号和密码。改桥接、关 TR-069 用得上。单个静态二进制，不用装运行环境。

[![Go](https://img.shields.io/badge/Go-1.22%2B-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Dependencies](https://img.shields.io/badge/dependencies-0-brightgreen.svg)](#技术取舍)
[![Platform](https://img.shields.io/badge/platform-windows%20%7C%20macOS%20%7C%20linux-lightgrey.svg)](#安装)

> Go 重写版。原始实现是 [joestar817/decrypt-lastgood](https://github.com/joestar817/decrypt-lastgood)。

## 背景

光猫后台只给普通账号，改桥接要用的超级管理员密码一般已被 TR-069 刷成随机值。九联这台
机器的超密存在 `/config/work/lastgood.xml`，新版固件又给文件加了密，`cat` 出来是二进制。
手工走 telnet、`base64`、本地解密太麻烦，所以有了这个工具。

## 特性

- 零第三方模块，PBKDF2 和 telnet 客户端都是标准库写的
- 编出来是单个文件，Windows 上只用系统自带的 `kernel32.dll`
- `--show-cred` 解完直接把超管账号密码打出来，不用自己 grep XML
- 对光猫只跑 `command -v`、`wc -c`、`base64` 三条只读命令，不改配置
- 下载后拿 `wc -c` 的字节数校验一遍
- Windows（x64 / ARM64）、macOS、Linux 都有

## 安装

从 [Releases](../../releases) 下载对应平台的压缩包，解压即用：

| 平台 | 压缩包 |
| --- | --- |
| Windows x64 | `dlg_<版本>_windows_amd64.zip` |
| Windows ARM64 | `dlg_<版本>_windows_arm64.zip` |
| macOS Apple Silicon | `dlg_<版本>_darwin_arm64.zip` |
| macOS Intel | `dlg_<版本>_darwin_amd64.zip` |
| Linux x64 | `dlg_<版本>_linux_amd64.zip` |

包里是二进制、README 和 LICENSE。macOS 和 Linux 解压后如果提示权限不足，`chmod +x` 一下。

需要自己编的话，Go 1.22 以上，不需要 cgo：

```bash
go build -trimpath -ldflags "-s -w" -o dlg ./cmd/dlg
./build.sh    # 全平台，输出到 dist/
```

## 前置条件

1. 用普通账号登录 `http://192.168.1.1`，账号密码印在机身标签上
2. 访问 `http://192.168.1.1/webcmcc/telnet_page.html` 开 telnet
3. 确认固件版本。V1.0.03 部分地区的 telnet 打不开
4. 用完把 telnet 关掉，它是明文调试端口

## 用法

```bash
dlg download 192.168.1.1 -u user -p '机身标签上的密码' -o lastgood.xml.enc
dlg decrypt lastgood.xml.enc --show-cred
```

输出：

```
文件已解密：/home/you/lastgood.xml

超级管理员账号：CMCCAdmin
超级管理员密码：xxxxxxxxxxxx
```

`download` 参数：

| 参数 | 说明 | 默认值 |
| --- | --- | --- |
| `-u` | telnet 用户名，必填 | — |
| `-p` | telnet 密码，省略则交互输入且不回显 | 交互输入 |
| `-P` | telnet 端口 | `23` |
| `-r` | 远端文件路径 | `/config/work/lastgood.xml` |
| `-t` | 连接超时（秒） | `8` |
| `-o` | 本地输出文件 | `lastgood.xml.enc` |
| `-v` | 输出连接与读取进度 | 关闭 |

`decrypt` 参数：`-o` 指定输出文件（默认 `lastgood.xml`），`--show-cred` 打印超管账号密码。

主机地址写在参数前或后都行。退出码：成功 `0`，运行时出错 `1`，参数用错 `2`。

## 工作原理

用普通账号 telnet 登录，在设备上跑 `command -v base64`、`wc -c`、`base64` 把文件带回来，
按字节数校验，然后三层密钥逐级解包、按 1040 字节分块 AES-CBC 解密，最后从 tar.gz 里取出
XML，读 `aucTeleAccountName` 和 `aucTeleAccountPassword`。

客户端要自己处理 telnet 选项协商，否则对端可能一直重试把连接卡住。

格式细节见 [docs/FORMAT.md](docs/FORMAT.md)。

## 测试

```bash
go test ./...
go test -race ./...
```

覆盖容器往返解密（含多分块）、各类畸形输入、telnet IAC 协商与跨包分片、CLI 参数解析。

不接光猫也能验证二进制：

```bash
DLG_FIXTURE_OUT=/tmp/fixture.enc go test ./internal/container -run TestWriteFixture
./dlg decrypt /tmp/fixture.enc --show-cred
```

## 兼容性

| 项目 | 状态 |
| --- | --- |
| 九联 `UNG953H1-S` | 开发和测试用的就是它 |
| 九联 `UNG953H-S` | 同系列，格式可能一样，没实测 |
| 其他厂商光猫 | 格式不同，用不了 |

V1.0.02 验证过。V1.0.03 有些批次开不了 telnet，跟工具本身无关。

## 与上游 Python 版的差异

同格式的独立实现，不是逐行翻译。

| 维度 | 上游 | 本项目 |
| --- | --- | --- |
| 运行环境 | Python 3.10–3.12 | 无，单个静态二进制 |
| 依赖 | `cryptography`、`telnetlib` | 无，两个都自己写 |
| 输出超密 | 手工 grep XML | `--show-cred` |
| 密码回显 | `getpass` | `SetConsoleMode` / `stty` |

容器解析、PBKDF2 参数、分块大小、远端只读命令、字节数校验都和上游保持一致。

## 技术取舍

PBKDF2 在 `golang.org/x/crypto` 里有现成的，但引一个模块进来，「零依赖单文件」就没了。
算法本身五十行左右，还能拿公开测试向量逐条对，自己写更好验证。telnet 同理，`telnetlib3`
是异步框架，而我只要「登录、跑三条命令、读回结果」，用它有点杀鸡用牛刀。

不支持别的厂商，是因为各家容器格式不一样，有用 ZIP 的，有另加一层密钥的。没有实机验证
就去做通用适配，最后只会给出一堆跑不通的承诺。

## 延伸阅读

- [docs/FORMAT.md](docs/FORMAT.md)：容器格式规范
- [docs/case-study-first-hop-packet-loss.md](docs/case-study-first-hop-packet-loss.md)：
  这工具的由来，一个用 ping 网关区分 CPU 过载和链路误码的案例

## 致谢与许可

格式的逆向工作来自 [joestar817/decrypt-lastgood](https://github.com/joestar817/decrypt-lastgood)，
恩山论坛（right.com.cn）的讨论提供了不少固件行为上的参考。本项目以
[MIT License](LICENSE) 发布。

上游没声明开源许可证。这边是基于公开逆向结论的独立实现，没有复制它的源代码；如果上游
之后补了许可证，以更严格的为准。

## 免责声明

只用于自己拥有或有权使用的设备。光猫一般是运营商资产，自己改可能影响保修和售后主张。
改桥接前记下 LOID、上网 VLAN ID 和宽带账号密码，不然断了网可能回不去。别用在没授权
的设备上。
