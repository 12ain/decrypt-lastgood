# 贡献指南

提 PR 之前有几件事先说清楚，省得来回改。

## 环境

Go 1.22 以上，不用 cgo，不用装第三方模块。

```bash
git clone https://github.com/12ain/decrypt-lastgood.git
cd decrypt-lastgood
go build ./...
```

## 提交前自查

```bash
gofmt -l .        # 应该没有输出
go vet ./...
go test -race ./...
```

三条都得过，gofmt 不过 CI 会直接拦。

## 约定

- 保持零依赖。要新增 `require`，请在 PR 里说明为什么标准库做不到
- 错误信息用小写英文，面向用户的中文文案放在 `internal/cli`
- 外部输入显式校验长度和边界，别指望 panic / recover 兜底
- 密钥、密码相关的变量不要进日志，也别出现在错误信息里
- 新功能带测试，修 bug 带上能复现的测试

包划分看一眼目录就清楚了：`cli` 管命令和文案，`container` 管解析解密，`kdf` 是 PBKDF2，
`telnet` 是客户端，`console` 是密码输入。

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
