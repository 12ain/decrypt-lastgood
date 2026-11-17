// Package cli wires the commands together.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/12ain/decrypt-lastgood/internal/console"
	"github.com/12ain/decrypt-lastgood/pkg/dlg"
)

// Version is reported by the version subcommand.
const Version = dlg.Version

// Run executes the CLI and returns a process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return 2
	}

	switch args[0] {
	case "download", "dl":
		return runDownload(args[1:], stdout, stderr)
	case "decrypt", "dec":
		return runDecrypt(args[1:], stdout, stderr)
	case "version", "-v", "--version":
		fmt.Fprintf(stdout, "dlg %s\n", Version)
		return 0
	case "help", "-h", "--help":
		usage(stdout)
		return 0
	}

	if !looksLikeHost(args[0]) {
		fmt.Fprintf(stderr, "error: unknown command %q\n\n", args[0])
		usage(stderr)
		return 2
	}

	// Anything that starts with a host is the one-shot command: fetch the
	// config, decrypt it and print the super administrator credentials.
	return runOneShot(args, stdout, stderr)
}

// fetchFlags is the flag set shared by the one-shot command and download.
type fetchFlags struct {
	user     *string
	password *string
	port     *int
	remote   *string
	timeout  *float64
	output   *string
	verbose  *bool
}

func registerFetchFlags(fs *flag.FlagSet, outputDefault, outputUsage string) *fetchFlags {
	return &fetchFlags{
		user:     fs.String("u", "", "telnet 用户名（必填，通常是机身标签上的普通账号）"),
		password: fs.String("p", "", "telnet 密码；省略则提示输入"),
		port:     fs.Int("P", dlg.DefaultPort, "telnet 端口"),
		remote:   fs.String("r", dlg.DefaultRemotePath, "远端文件路径"),
		timeout:  fs.Float64("t", dlg.DefaultTimeout.Seconds(), "连接超时（秒）"),
		output:   fs.String("o", outputDefault, outputUsage),
		verbose:  fs.Bool("v", false, "打印完整 telnet 会话记录"),
	}
}

func (f *fetchFlags) valueFlags() map[string]bool {
	return map[string]bool{"u": true, "p": true, "P": true, "r": true, "t": true, "o": true}
}

// resolve validates the user name and asks for the password when it was omitted.
func (f *fetchFlags) resolve(stderr io.Writer) (string, error) {
	if *f.user == "" {
		return "", errors.New("必须通过 -u 指定 telnet 用户名")
	}
	if *f.password != "" {
		return *f.password, nil
	}

	fmt.Fprint(stderr, "Telnet password: ")
	secret, err := console.ReadLine()
	fmt.Fprintln(stderr)
	if err != nil {
		return "", err
	}
	return secret, nil
}

// config is the device and the session to it, as the flags describe them.
func (f *fetchFlags) config(host, secret string, stderr io.Writer) dlg.Config {
	cfg := dlg.Config{
		Host:       host,
		Port:       *f.port,
		User:       *f.user,
		Password:   secret,
		RemotePath: *f.remote,
		Timeout:    time.Duration(*f.timeout * float64(time.Second)),
	}
	// The transcript has to start before login, otherwise the most
	// interesting part of a failed session is missing from -v output.
	if *f.verbose {
		fmt.Fprintf(stderr, "正在连接 %s:%d ...\n----- telnet 会话（>> 发送 << 接收）-----\n", host, *f.port)
		cfg.Transcript = stderr
	}
	return cfg
}

// report prints the steps of a fetch when -v asked for them.
func (f *fetchFlags) report(stderr io.Writer) func(dlg.Progress) {
	if !*f.verbose {
		return nil
	}
	return func(p dlg.Progress) {
		if p.Stage == dlg.StageDownload {
			fmt.Fprintf(stderr, "正在读取 %s ...\n", p.Detail)
		}
	}
}

// fetch logs in and returns the encrypted config, with the -v transcript
// attached when requested.
func (f *fetchFlags) fetch(host, secret string, stderr io.Writer) ([]byte, error) {
	return dlg.Fetch(f.config(host, secret, stderr), f.report(stderr))
}

func runOneShot(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("dlg", flag.ContinueOnError)
	fs.SetOutput(stderr)

	flags := registerFetchFlags(fs, dlg.DefaultOutput, "解密后的输出文件")

	host, err := parse(fs, args, flags.valueFlags(), 1)
	if err != nil {
		return fail(stderr, err)
	}

	secret, err := flags.resolve(stderr)
	if err != nil {
		return fail(stderr, err)
	}

	cfg := flags.config(host, secret, stderr)
	encrypted, err := dlg.Fetch(cfg, flags.report(stderr))
	if err != nil {
		return failWithHint(stderr, err, host)
	}

	plain, err := dlg.Decrypt(encrypted)
	if err != nil {
		return fail(stderr, err)
	}

	if err := dlg.WriteFile(*flags.output, plain); err != nil {
		return fail(stderr, err)
	}

	cred := dlg.ReadCredential(plain)
	if !cred.Complete() {
		fmt.Fprintln(stderr, "未能在配置文件中定位超级管理员账号（aucTeleAccountName / aucTeleAccountPassword）")
		fmt.Fprintf(stdout, "配置文件：%s\n", dlg.AbsolutePath(*flags.output))
		return 1
	}

	printCredential(stdout, cred)
	fmt.Fprintf(stdout, "\n配置文件：%s\n", dlg.AbsolutePath(*flags.output))
	return 0
}

func runDownload(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("download", flag.ContinueOnError)
	fs.SetOutput(stderr)

	flags := registerFetchFlags(fs, dlg.DefaultEncryptedOutput, "本地输出文件")

	host, err := parse(fs, args, flags.valueFlags(), 1)
	if err != nil {
		return fail(stderr, err)
	}

	secret, err := flags.resolve(stderr)
	if err != nil {
		return fail(stderr, err)
	}

	data, err := flags.fetch(host, secret, stderr)
	if err != nil {
		return failWithHint(stderr, err, host)
	}

	if err := dlg.WriteFile(*flags.output, data); err != nil {
		return fail(stderr, err)
	}
	fmt.Fprintf(stdout, "文件已下载：%s\n", dlg.AbsolutePath(*flags.output))
	fmt.Fprintf(stdout, "解密：dlg decrypt %s --show-cred\n", *flags.output)
	return 0
}

func runDecrypt(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("decrypt", flag.ContinueOnError)
	fs.SetOutput(stderr)

	output := fs.String("o", dlg.DefaultOutput, "解密后的输出文件")
	showCred := fs.Bool("show-cred", false, "解密后直接打印超级管理员账号密码")

	input, err := parse(fs, args, map[string]bool{"o": true}, 1)
	if err != nil {
		return fail(stderr, err)
	}

	encrypted, err := dlg.ReadFile(input)
	if err != nil {
		return fail(stderr, err)
	}

	plain, err := dlg.Decrypt(encrypted)
	if err != nil {
		return fail(stderr, err)
	}

	if err := dlg.WriteFile(*output, plain); err != nil {
		return fail(stderr, err)
	}
	fmt.Fprintf(stdout, "文件已解密：%s\n", dlg.AbsolutePath(*output))

	if *showCred {
		cred := dlg.ReadCredential(plain)
		if !cred.Complete() {
			fmt.Fprintln(stderr, "未能在配置文件中定位超级管理员账号（aucTeleAccountName / aucTeleAccountPassword）")
			return 1
		}
		fmt.Fprintln(stdout)
		printCredential(stdout, cred)
	}

	return 0
}

func printCredential(w io.Writer, cred dlg.Credential) {
	fmt.Fprintf(w, "超级管理员账号：%s\n超级管理员密码：%s\n", cred.User, cred.Password)
}

// looksLikeHost keeps typos ("dlg donwload ...") from being taken for a device
// address: the default command only fires on something that has a dot or a
// colon. Devices are addressed by IP in practice.
func looksLikeHost(arg string) bool {
	return arg == "localhost" || strings.ContainsAny(arg, ".:")
}

func printTelnetHint(w io.Writer, host string) {
	fmt.Fprintf(w, `
连不上 23 端口，通常是光猫的 Telnet 还没开。先做这一步：
  1. 浏览器打开 %s
  2. 用机身标签上的普通账号登录，把 Telnet 开关打开并保存
  3. 重新执行本命令

`, dlg.TelnetPageURL(host))
}

// parse splits a mix of positional arguments and flags, verifies the
// positional count and parses the flags. Hosts are conventionally written
// before the flags, which the stdlib flag package cannot handle on its own.
func parse(fs *flag.FlagSet, args []string, valueFlags map[string]bool, want int) (string, error) {
	positional, flags := partition(args, valueFlags)

	if err := fs.Parse(flags); err != nil {
		return "", err
	}
	if len(positional) != want {
		return "", fmt.Errorf("需要 %d 个位置参数，实际收到 %d 个", want, len(positional))
	}
	return positional[0], nil
}

func partition(args []string, valueFlags map[string]bool) (positional, flags []string) {
	for i := 0; i < len(args); i++ {
		arg := args[i]

		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positional = append(positional, arg)
			continue
		}

		flags = append(flags, arg)

		name := strings.TrimLeft(arg, "-")
		if strings.ContainsRune(name, '=') {
			continue
		}
		if valueFlags[name] && i+1 < len(args) {
			flags = append(flags, args[i+1])
			i++
		}
	}
	return positional, flags
}

// failWithHint reports a fetch failure and, when the device could not be
// reached at all, explains how to turn telnet on.
func failWithHint(stderr io.Writer, err error, host string) int {
	code := fail(stderr, err)
	if dlg.IsConnectError(err) {
		printTelnetHint(stderr, host)
	}
	return code
}

func fail(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "error: %v\n", err)
	if errors.Is(err, flag.ErrHelp) {
		return 2
	}
	return 1
}

func usage(w io.Writer) {
	fmt.Fprint(w, `dlg - 一条命令取回并解密光猫配置，输出超级管理员账号密码

前置步骤（只做一次，Telnet 默认是关的）：
  1. 浏览器打开 http://192.168.1.1/webcmcc/telnet_page.html
     （把 192.168.1.1 换成你的光猫地址）
  2. 用机身标签上的普通账号登录，打开 Telnet 开关并保存

用法：
  dlg <光猫IP> -u <用户名> [-p <密码>] [-o <文件>]   默认：下载 + 解密 + 输出账号密码
  dlg download <光猫IP> -u <用户名> [-o <文件>]      只下载加密文件
  dlg decrypt <加密文件> [-o <文件>] [--show-cred]   解密本地文件
  dlg version

参数：
  -u    telnet 用户名（必填，通常是机身标签上的普通账号）
  -p    telnet 密码；省略则交互输入
  -P    telnet 端口，默认 23
  -r    远端文件路径，默认 /config/work/lastgood.xml
  -t    连接超时秒数，默认 8
  -o    输出文件：默认命令写解密后的 XML（默认 lastgood.xml），
        download 写加密文件（默认 lastgood.xml.enc）
  -v    打印完整 telnet 会话记录，排查用

decrypt 参数：
  --show-cred  解密后直接打印超级管理员账号密码

图形界面：同一仓库的 gui/ 是个原生窗口客户端，填地址和账号点一下就行，
不用记参数。见 docs/GUI.md。

示例：
  dlg 192.168.1.1 -u user -p 标签上的密码
  dlg download 192.168.1.1 -u user
  dlg decrypt lastgood.xml.enc --show-cred
`)
}
