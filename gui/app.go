package main

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"

	"github.com/12ain/decrypt-lastgood/pkg/dlg"
)

// The pages of the tab bar.
const (
	tabFetch = iota
	tabDownload
	tabDecrypt
)

// maxLogLines bounds what the window keeps: a verbose session is long, and the
// text area lays out everything it holds.
const maxLogLines = 400

// operations is the work the window starts, replaceable in tests so that no
// test talks to a device.
type operations struct {
	fetch    func(cfg dlg.Config, output string, report func(dlg.Progress)) (dlg.Result, error)
	download func(cfg dlg.Config, output string, report func(dlg.Progress)) (string, error)
	decrypt  func(input, output string, report func(dlg.Progress)) (dlg.Result, error)
}

func defaultOperations() operations {
	return operations{
		fetch: dlg.FetchAndDecrypt,
		download: func(cfg dlg.Config, output string, report func(dlg.Progress)) (string, error) {
			if output == "" {
				output = dlg.DefaultEncryptedOutput
			}
			data, err := dlg.Fetch(cfg, report)
			if err != nil {
				return "", err
			}
			return dlg.SaveEncrypted(data, output)
		},
		decrypt: dlg.DecryptFile,
	}
}

// app is the state the window shows: the form as the user left it, and what
// the last run produced.
type app struct {
	// The form.
	host     string
	user     string
	password string
	port     string
	timeout  string
	remote   string
	output   string
	input    string
	verbose  bool
	reveal   bool
	tab      int

	// The run.
	busy      bool
	stage     string
	progress  float64
	started   time.Time
	elapsed   time.Duration
	cred      dlg.Credential
	savedPath string
	errText   string
	hintURL   string
	logText   string
	log       *logBuffer

	// Wiring: the window, and how work is run. Both change in tests: there is
	// no window and no goroutine, so a test is deterministic.
	window     *mygo.Window
	spawn      func(func())
	post       func(func())
	ops        operations
	openDialog func(current string) (string, error)
	saveDialog func(current string) (string, error)
}

func newApp() *app {
	a := &app{
		host:    "192.168.1.1",
		user:    "user",
		port:    strconv.Itoa(dlg.DefaultPort),
		timeout: strconv.Itoa(int(dlg.DefaultTimeout.Seconds())),
		remote:  dlg.DefaultRemotePath,
		output:  dlg.DefaultOutputPath(),
		ops:     defaultOperations(),
	}
	// Work runs on its own goroutine, as slow network I/O must; its results
	// reach the view through Window.Update, which runs them on the main thread.
	a.spawn = func(fn func()) { go fn() }
	a.post = func(fn func()) {
		if a.window != nil {
			a.window.Update(fn)
			return
		}
		fn()
	}
	a.log = &logBuffer{emit: func(line string) {
		a.post(func() { a.appendLog(line) })
	}}
	a.openDialog = openFileDialog
	a.saveDialog = saveFileDialog
	return a
}

// pickFile asks the system for a path and puts it in the field. The dialog
// blocks, so it runs off the main thread; the answer comes back to the view
// through post.
func (a *app) pickFile(target *string, forOpen bool) {
	current := *target
	a.spawn(func() {
		choose := a.saveDialog
		if forOpen {
			choose = a.openDialog
		}
		path, err := choose(current)
		if err != nil || path == "" {
			return
		}
		a.post(func() { *target = path })
	})
}

// attach gives the app its window, which publishes results from other
// goroutines to the view.
func (a *app) attach(w *mygo.Window) { a.window = w }

// connection is the device as the form describes it.
func (a *app) connection() (dlg.Config, error) {
	cfg := dlg.Config{
		Host:       strings.TrimSpace(a.host),
		User:       strings.TrimSpace(a.user),
		Password:   a.password,
		RemotePath: strings.TrimSpace(a.remote),
	}

	if v := strings.TrimSpace(a.port); v != "" {
		port, err := strconv.Atoi(v)
		if err != nil || port < 1 || port > 65535 {
			return cfg, fmt.Errorf("端口 %q 不是 1 到 65535 之间的数字", v)
		}
		cfg.Port = port
	}

	if v := strings.TrimSpace(a.timeout); v != "" {
		seconds, err := strconv.ParseFloat(v, 64)
		if err != nil || seconds <= 0 {
			return cfg, fmt.Errorf("超时 %q 不是正数秒数", v)
		}
		cfg.Timeout = time.Duration(seconds * float64(time.Second))
	}

	return cfg, cfg.Validate()
}

// outputPath is where the decrypted XML goes.
func (a *app) outputPath() string {
	if v := strings.TrimSpace(a.output); v != "" {
		return v
	}
	return dlg.DefaultOutputPath()
}

// start runs the page the user is on: fetch and decrypt, download only, or
// decrypt a file already on disk.
func (a *app) start() {
	if a.busy {
		return
	}

	output := a.outputPath()

	if a.tab == tabDecrypt {
		input := strings.TrimSpace(a.input)
		if input == "" {
			a.reject("先选一个 lastgood.xml.enc 文件。")
			return
		}
		a.begin("正在解密")
		a.spawn(func() {
			res, err := a.ops.decrypt(input, output, a.report)
			a.complete(res, err)
		})
		return
	}

	cfg, err := a.connection()
	if err != nil {
		a.reject(err.Error())
		return
	}
	// The transcript starts before login, so a failed session shows why.
	cfg.Transcript = a.log

	if a.tab == tabDownload {
		a.begin("正在连接设备")
		a.spawn(func() {
			path, err := a.ops.download(cfg, output, a.report)
			a.completeDownload(path, err)
		})
		return
	}

	a.begin("正在连接设备")
	a.spawn(func() {
		res, err := a.ops.fetch(cfg, output, a.report)
		a.complete(res, err)
	})
}

// takeDropped switches to the page that decrypts a file the user dropped on
// the window.
func (a *app) takeDropped(path string) {
	if a.busy || path == "" {
		return
	}
	a.input = path
	a.tab = tabDecrypt
	a.errText = ""
	a.stage = ""
	a.appendLog(clock() + "  已选择 " + path)
}

// begin resets what the window shows and marks the run as in progress.
func (a *app) begin(stage string) {
	a.busy = true
	a.errText = ""
	a.hintURL = ""
	a.cred = dlg.Credential{}
	a.savedPath = ""
	a.reveal = false
	a.stage = stage
	a.progress = -1
	a.started = time.Now()
	a.logText = ""
	a.log.reset()
	a.appendLog(clock() + "  " + stage)
}

// report is called from the working goroutine as each step starts.
func (a *app) report(p dlg.Progress) {
	text := describe(p)
	a.post(func() {
		if !a.busy {
			return
		}
		a.stage = text
		a.progress = p.Fraction
		a.appendLog(clock() + "  " + text)
	})
}

func (a *app) complete(res dlg.Result, err error) {
	a.post(func() {
		a.busy = false
		a.elapsed = time.Since(a.started)
		if err != nil {
			a.fail(err)
			return
		}

		a.cred = res.Credential
		a.savedPath = res.Path
		a.stage = "完成"
		a.progress = 1

		if res.Credential.Complete() {
			a.appendLog(clock() + "  已取得超级管理员账号")
		} else {
			a.appendLog(clock() + "  配置文件里没有 aucTeleAccountName / aucTeleAccountPassword")
			a.errText = "文件已解密并保存到 " + res.Path + "，但里面没有超级管理员账号。" +
				"这台设备可能不是九联的光猫，或者固件把这几个字段改了名。"
		}
		a.appendLog(fmt.Sprintf("%s  完成，用时 %s", clock(), a.elapsed.Round(100*time.Millisecond)))
	})
}

func (a *app) completeDownload(path string, err error) {
	a.post(func() {
		a.busy = false
		a.elapsed = time.Since(a.started)
		if err != nil {
			a.fail(err)
			return
		}
		a.savedPath = path
		a.stage = "完成"
		a.progress = 1
		a.appendLog(fmt.Sprintf("%s  完成，用时 %s", clock(), a.elapsed.Round(100*time.Millisecond)))
	})
}

// fail shows what went wrong, and how to turn telnet on when that was the
// problem.
func (a *app) fail(err error) {
	a.stage = "失败"
	a.progress = 0
	a.errText = err.Error()
	a.appendLog(clock() + "  失败：" + err.Error())

	if dlg.IsConnectError(err) {
		a.hintURL = dlg.TelnetPageURL(strings.TrimSpace(a.host))
		a.errText = "连不上设备的 telnet 端口。光猫出厂默认关着它，先把开关打开再试。"
	}
}

// reject reports a problem with the form itself, before any work starts.
func (a *app) reject(message string) {
	a.busy = false
	a.stage = ""
	a.progress = 0
	a.errText = message
	a.appendLog(clock() + "  " + message)
}

func (a *app) appendLog(line string) {
	if a.logText == "" {
		a.logText = line
	} else {
		a.logText += "\n" + line
	}
	// Keep the end, which is where the session failed.
	if n := strings.Count(a.logText, "\n") + 1; n > maxLogLines {
		lines := strings.SplitN(a.logText, "\n", n-maxLogLines+1)
		a.logText = lines[len(lines)-1]
	}
}

// describe is the line a step shows.
func describe(p dlg.Progress) string {
	text := dlg.DescribeStage(p.Stage)
	if p.Detail != "" {
		return text + "：" + p.Detail
	}
	return text
}

func clock() string { return time.Now().Format("15:04:05") }

// logBuffer collects the telnet transcript into lines, which reach the view
// one by one as the session goes on.
type logBuffer struct {
	mu      sync.Mutex
	partial string
	emit    func(string)
}

func (l *logBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.partial += string(p)
	for {
		i := strings.IndexByte(l.partial, '\n')
		if i < 0 {
			break
		}
		line := strings.TrimRight(l.partial[:i], "\r")
		l.partial = l.partial[i+1:]
		if strings.TrimSpace(line) != "" {
			l.emit(line)
		}
	}
	return len(p), nil
}

// reset drops a half-written line, so that a new run starts clean.
func (l *logBuffer) reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.partial = ""
}
