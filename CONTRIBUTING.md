# 贡献指南

提 PR 之前有几件事先说清楚，省得来回改。

## 环境

整个仓库一个 module，Go 1.27.3 以上（`go.mod` 里的版本，窗口版用的 MyGo 要求的），不用 cgo。

模块里唯一的第三方依赖是 MyGo，只被 `gui` 用到。命令行那边不链接它，所以编出来的 `dlg`
仍然是不带第三方代码的静态二进制。

```bash
git clone https://github.com/12ain/decrypt-lastgood.git
cd decrypt-lastgood
go build ./...      # 命令行与窗口版一起
go build ./cmd/dlg  # 只要命令行
```

## 提交前自查

```bash
gofmt -l .        # 应该没有输出
go vet ./...
go test -race ./...

cd gui && go tool mygo vet .
```

都得过，gofmt 不过 CI 会直接拦。`go tool mygo vet` 查的是原生界面的老问题：把 build 期的
元素存进结构体或 goroutine。窗口版的测试在 macOS 上跑（CI 就是这么分的），本地在任意平台
`go test ./gui` 都行，前提是那个平台能加载字体。

## 约定

- 命令行的导入图里不能出现第三方包（`go list -deps ./cmd/dlg ./pkg/dlg` 自查）；要给窗口版
  加依赖，请在 PR 里说明理由，并确认 `dlg` 的体积和依赖没变
- 错误信息用小写英文；面向用户的中文文案集中在 `pkg/dlg`（两边共用）和 `gui/view.go`
  （窗口自己的措辞），命令行参数说明留在 `internal/cli`
- 外部输入显式校验长度和边界，别指望 panic / recover 兜底
- 密钥、密码相关的变量不要进日志，也别出现在错误信息里
- 新功能带测试，修 bug 带上能复现的测试
- 界面改动顺手跑一遍 `cd gui && GUI_SNAPSHOT=/tmp/dlg go test . -run TestSnapshot`，
  看一眼 `/tmp/dlg-*.png`，深浅色都要看

包划分看一眼目录就清楚了：`gui` 是窗口，`cli` 管命令和文案，`pkg/dlg` 是两边共用的库层，
`container` 管解析解密，`kdf` 是 PBKDF2，`telnet` 是客户端，`console` 是密码输入。

改 `gui/` 时别把 `ui.Element` 存进状态：它只属于当前这一帧，下一帧就失效了。慢 IO 丢到
`app.spawn` 起的 goroutine 里，结果用 `app.post` 回主线程。细节见
[docs/GUI.md](docs/GUI.md#视图怎么写的)。

## 提交信息

用 [Conventional Commits](https://www.conventionalcommits.org/zh-hans/)，格式
`<type>(<scope>): <subject>`，正文写清楚为什么要改，diff 里有的不用重复。

```
fix(telnet): 修正跨包分片时 IAC 状态机丢字节

IAC 序列被 TCP 切开时，状态机在第二个分片里会被重置，后续数据被误判成
协商命令丢掉。改成把状态挂在 Client 上。
```

## PR

一个 PR 只做一件事，别顺手带上无关的格式化改动。描述里说清楚解决什么问题、怎么验证的、
有没有副作用。动到格式解析的话，顺带判断 `docs/FORMAT.md` 要不要同步改。

合并走 Squash and Merge，所以 PR 标题会直接变成主干上那条提交的标题，按「这条就是最终
提交」来写。

## 能提什么，别贴什么

欢迎：其他固件版本的兼容性反馈、同系列机型能不能用、解密失败卡在哪一步。

不要往 issue 或 PR 里粘完整密钥材料、解密后的 `lastgood.xml` 全文，或者你自己的超管密码、
LOID、宽带账号。给「文件总字节数 + 卡在哪一步 + 报错原文」就够定位了。

## 许可

提了贡献，就视为同意按本项目的 [MIT License](LICENSE) 授权。
