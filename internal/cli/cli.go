// Package cli wires the commands together.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/12ain/decrypt-lastgood/internal/console"
	"github.com/12ain/decrypt-lastgood/internal/container"
	"github.com/12ain/decrypt-lastgood/internal/telnet"
)

// Version is reported by the version subcommand.
const Version = "0.1.0"

const defaultRemotePath = "/config/work/lastgood.xml"

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
	default:
		fmt.Fprintf(stderr, "error: unknown command %q\n\n", args[0])
		usage(stderr)
		return 2
	}
}

func runDownload(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("download", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		user     = fs.String("u", "", "telnet 用户名（必填）")
		password = fs.String("p", "", "telnet 密码；省略则提示输入")
		port     = fs.Int("P", 23, "telnet 端口")
		remote   = fs.String("r", defaultRemotePath, "远端文件路径")
		timeout  = fs.Float64("t", 8, "连接超时（秒）")
		output   = fs.String("o", "lastgood.xml.enc", "本地输出文件")
		verbose  = fs.Bool("v", false, "输出连接过程")
	)

	host, err := parse(fs, args, map[string]bool{"u": true, "p": true, "P": true, "r": true, "t": true, "o": true}, 1)
	if err != nil {
		return fail(stderr, err)
	}
	if *user == "" {
		return fail(stderr, errors.New("必须通过 -u 指定 telnet 用户名"))
	}

	secret := *password
	if secret == "" {
		fmt.Fprint(stderr, "Telnet password: ")
		secret, err = console.ReadLine()
		fmt.Fprintln(stderr)
		if err != nil {
			return fail(stderr, err)
		}
	}

	if *verbose {
		fmt.Fprintf(stderr, "正在连接 %s:%d ...\n", host, *port)
	}

	client, err := telnet.Dial(telnet.Target{
		Host:    host,
		Port:    *port,
		User:    *user,
		Pass:    secret,
		Timeout: time.Duration(*timeout * float64(time.Second)),
	})
	if err != nil {
		return fail(stderr, err)
	}
	defer client.Close()

	if *verbose {
		fmt.Fprintf(stderr, "正在读取 %s ...\n", *remote)
	}

	data, err := client.FetchFile(*remote)
	if err != nil {
		return fail(stderr, err)
	}

	if err := os.WriteFile(*output, data, 0o600); err != nil {
		return fail(stderr, err)
	}
	fmt.Fprintf(stdout, "文件已下载：%s\n", absolute(*output))
	return 0
}

func runDecrypt(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("decrypt", flag.ContinueOnError)
	fs.SetOutput(stderr)

	output := fs.String("o", "lastgood.xml", "解密后的输出文件")
	showCred := fs.Bool("show-cred", false, "解密后直接打印超级管理员账号密码")

	input, err := parse(fs, args, map[string]bool{"o": true}, 1)
	if err != nil {
		return fail(stderr, err)
	}

	encrypted, err := os.ReadFile(input)
	if err != nil {
		return fail(stderr, err)
	}

	plain, err := container.DecryptXML(encrypted)
	if err != nil {
		return fail(stderr, err)
	}

	if err := os.WriteFile(*output, plain, 0o600); err != nil {
		return fail(stderr, err)
	}
	fmt.Fprintf(stdout, "文件已解密：%s\n", absolute(*output))

	if *showCred {
		cred := container.FindCredential(plain)
		if !cred.Complete() {
			fmt.Fprintln(stderr, "未能在配置文件中定位超级管理员账号（aucTeleAccountName / aucTeleAccountPassword）")
			return 1
		}
		fmt.Fprintf(stdout, "\n超级管理员账号：%s\n超级管理员密码：%s\n", cred.User, cred.Password)
	}

	return 0
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

func absolute(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

func fail(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "error: %v\n", err)
	if errors.Is(err, flag.ErrHelp) {
		return 2
	}
	return 1
}

func usage(w io.Writer) {
	fmt.Fprint(w, `dlg - 下载并解密光猫 lastgood.xml 配置备份

用法：
  dlg download <光猫IP> -u <用户名> [-p <密码>] [-o <输出>] [-v]
  dlg decrypt <加密文件> [-o <输出>] [--show-cred]
  dlg version

download 参数：
  -u    telnet 用户名（必填，通常是 user）
  -p    telnet 密码；省略则交互输入
  -P    telnet 端口，默认 23
  -r    远端文件路径，默认 /config/work/lastgood.xml
  -t    连接超时秒数，默认 8
  -o    本地输出文件，默认 lastgood.xml.enc

decrypt 参数：
  -o          解密后的输出文件，默认 lastgood.xml
  --show-cred 解密后直接打印超级管理员账号密码

示例：
  dlg download 192.168.1.1 -u user -p 标签上的密码 -o lastgood.xml.enc
  dlg decrypt lastgood.xml.enc --show-cred
`)
}
