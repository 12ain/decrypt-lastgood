package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/12ain/decrypt-lastgood/pkg/dlg"
)

// newTestApp is the window with the work stubbed out and everything
// synchronous: a click runs the whole job before the tester draws again.
func newTestApp(t *testing.T, ops operations) *app {
	t.Helper()

	a := newApp()
	a.spawn = func(fn func()) { fn() }
	a.post = func(fn func()) { fn() }
	a.openDialog = func(string) (string, error) { return "", errors.New("no dialog in tests") }
	a.saveDialog = func(string) (string, error) { return "", errors.New("no dialog in tests") }
	if ops.fetch != nil || ops.download != nil || ops.decrypt != nil {
		a.ops = ops
	}
	return a
}

// failingOps is what a device that cannot be reached produces.
func failingOps(err error) operations {
	return operations{
		fetch:    func(dlg.Config, string, func(dlg.Progress)) (dlg.Result, error) { return dlg.Result{}, err },
		download: func(dlg.Config, string, func(dlg.Progress)) (string, error) { return "", err },
		decrypt:  func(string, string, func(dlg.Progress)) (dlg.Result, error) { return dlg.Result{}, err },
	}
}

func TestViewShowsTheForm(t *testing.T) {
	a := newTestApp(t, operations{})
	tt := ui.NewTester(a.view, 960, 700)

	for _, text := range []string{"光猫超级管理员密码", "获取凭据", "仅下载", "解密本地文件", "获取超级管理员密码"} {
		if !tt.HasText(text) {
			t.Errorf("missing %q in %q", text, tt.Texts())
		}
	}
	if a.host != "192.168.1.1" {
		t.Errorf("host = %q, want the usual modem address", a.host)
	}
}

func TestFetchShowsTheCredential(t *testing.T) {
	ops := operations{fetch: func(cfg dlg.Config, output string, report func(dlg.Progress)) (dlg.Result, error) {
		if cfg.Host != "192.168.1.1" || cfg.User != "user" || cfg.Port != dlg.DefaultPort {
			t.Errorf("config = %+v", cfg)
		}
		if output == "" {
			t.Error("no output path")
		}
		report(dlg.Progress{Stage: dlg.StageConnect, Detail: cfg.Address(), Fraction: -1})
		report(dlg.Progress{Stage: dlg.StageDownload, Detail: cfg.RemotePath, Fraction: -1})
		return dlg.Result{
			Credential: dlg.Credential{User: "superadmin", Password: "s3cret"},
			Path:       "/tmp/lastgood.xml",
		}, nil
	}}
	a := newTestApp(t, ops)
	tt := ui.NewTester(a.view, 960, 700)

	if err := tt.Click("获取超级管理员密码"); err != nil {
		t.Fatal(err)
	}
	if a.busy {
		t.Error("still busy after the job finished")
	}
	if !tt.HasText("superadmin") {
		t.Errorf("the account is not shown: %q", tt.Texts())
	}
	if tt.HasText("s3cret") {
		t.Error("the password shows before the user asks for it")
	}
	if !tt.HasText("已保存：/tmp/lastgood.xml") {
		t.Errorf("the saved path is not shown: %q", tt.Texts())
	}

	if err := tt.Click("显示"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("s3cret") {
		t.Errorf("the password did not show: %q", tt.Texts())
	}
}

func TestFetchCopiesThePassword(t *testing.T) {
	a := newTestApp(t, operations{fetch: func(dlg.Config, string, func(dlg.Progress)) (dlg.Result, error) {
		return dlg.Result{Credential: dlg.Credential{User: "superadmin", Password: "s3cret"}, Path: "/tmp/x.xml"}, nil
	}})
	tt := ui.NewTester(a.view, 960, 700)

	if err := tt.Click("获取超级管理员密码"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("复制密码"); err != nil {
		t.Fatal(err)
	}
	if got := tt.Clipboard(); got != "s3cret" {
		t.Fatalf("clipboard = %q", got)
	}
}

func TestConnectErrorPointsAtTheTelnetPage(t *testing.T) {
	a := newTestApp(t, failingOps(fmt.Errorf("%w：connect to 192.168.1.1:23: connection refused", dlg.ErrConnect)))
	tt := ui.NewTester(a.view, 960, 700)

	if err := tt.Click("获取超级管理员密码"); err != nil {
		t.Fatal(err)
	}
	if a.errText == "" || a.hintURL != "http://192.168.1.1/webcmcc/telnet_page.html" {
		t.Fatalf("error = %q, hint = %q", a.errText, a.hintURL)
	}
	if !tt.HasText("打开开启 Telnet 的页面") {
		t.Errorf("no hint link: %q", tt.Texts())
	}
	if len(tt.OpenedURLs()) != 0 {
		t.Fatalf("nothing was clicked yet, urls = %v", tt.OpenedURLs())
	}
	if err := tt.Click("打开开启 Telnet 的页面"); err != nil {
		t.Fatal(err)
	}
	if len(tt.OpenedURLs()) == 0 {
		t.Fatal("the link did not open in the system")
	}
}

func TestOtherErrorsShowTheMessage(t *testing.T) {
	a := newTestApp(t, failingOps(errors.New("telnet login failed")))
	tt := ui.NewTester(a.view, 960, 700)

	if err := tt.Click("获取超级管理员密码"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("telnet login failed") {
		t.Errorf("error not shown: %q", tt.Texts())
	}
	if a.hintURL != "" {
		t.Errorf("hint = %q, want none for a login failure", a.hintURL)
	}
}

func TestFormIsValidatedBeforeConnecting(t *testing.T) {
	called := false
	a := newTestApp(t, operations{fetch: func(dlg.Config, string, func(dlg.Progress)) (dlg.Result, error) {
		called = true
		return dlg.Result{}, nil
	}})
	a.host = "  "
	tt := ui.NewTester(a.view, 960, 700)

	if err := tt.Click("获取超级管理员密码"); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("the job ran without an address")
	}
	if !tt.HasText("必须指定光猫地址") {
		t.Errorf("no message about the address: %q", tt.Texts())
	}

	a.host, a.port = "192.168.1.1", "abc"
	tt.Frame()
	if err := tt.Click("获取超级管理员密码"); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("the job ran with a port that is not a number")
	}
	if !tt.HasText(`端口 "abc" 不是 1 到 65535 之间的数字`) {
		t.Errorf("no message about the port: %q", tt.Texts())
	}
}

func TestDownloadModeSavesTheEncryptedFile(t *testing.T) {
	a := newTestApp(t, operations{download: func(dlg.Config, string, func(dlg.Progress)) (string, error) {
		return "/tmp/lastgood.xml.enc", nil
	}})
	tt := ui.NewTester(a.view, 960, 700)

	if err := tt.Click("仅下载"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("只下载加密文件") {
		t.Fatalf("the download page is not shown: %q", tt.Texts())
	}
	if err := tt.Click("只下载加密文件"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("已保存：/tmp/lastgood.xml.enc") {
		t.Errorf("the saved file is not shown: %q", tt.Texts())
	}
}

func TestDecryptPageNeedsAFile(t *testing.T) {
	a := newTestApp(t, operations{decrypt: func(input, output string, report func(dlg.Progress)) (dlg.Result, error) {
		if input != "/tmp/in.enc" {
			t.Errorf("input = %q", input)
		}
		return dlg.Result{
			Credential: dlg.Credential{User: "superadmin", Password: "s3cret"},
			Path:       output,
		}, nil
	}})
	tt := ui.NewTester(a.view, 960, 700)

	if err := tt.Click("解密本地文件"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("解密这个文件") {
		t.Fatalf("the decrypt page is not shown: %q", tt.Texts())
	}
	if err := tt.Click("解密这个文件"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("先选一个 lastgood.xml.enc 文件。") {
		t.Errorf("no message about the missing file: %q", tt.Texts())
	}

	a.input = "/tmp/in.enc"
	tt.Frame()
	if err := tt.Click("解密这个文件"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("superadmin") {
		t.Errorf("the account is not shown: %q", tt.Texts())
	}
	if tt.HasText("光猫地址") {
		t.Error("the connection fields still show on the decrypt page")
	}
}

// TestDroppingAFileOpensItOnTheDecryptPage covers the shortcut for the second
// half of the job: the user already has the encrypted file.
func TestDroppingAFileOpensItOnTheDecryptPage(t *testing.T) {
	a := newTestApp(t, operations{decrypt: func(input, output string, report func(dlg.Progress)) (dlg.Result, error) {
		if input != "/tmp/dropped.xml.enc" {
			t.Errorf("input = %q", input)
		}
		return dlg.Result{Credential: dlg.Credential{User: "superadmin", Password: "s3cret"}, Path: output}, nil
	}})
	tt := ui.NewTester(a.view, 960, 700)

	if !tt.DropFiles(480, 350, "/tmp/dropped.xml.enc") {
		t.Fatal("the window did not take the dropped file")
	}
	if a.tab != tabDecrypt || a.input != "/tmp/dropped.xml.enc" {
		t.Fatalf("tab = %d, input = %q", a.tab, a.input)
	}
	if !tt.HasText("解密这个文件") {
		t.Errorf("the decrypt page is not shown: %q", tt.Texts())
	}
	if err := tt.Click("解密这个文件"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("superadmin") {
		t.Errorf("the account is not shown: %q", tt.Texts())
	}
}

// TestVerboseTranscriptReachesTheLog checks the wiring the window needs: the
// telnet client writes to cfg.Transcript, a logBuffer turns that into lines,
// and they land in the text area.
func TestVerboseTranscriptReachesTheLog(t *testing.T) {
	a := newTestApp(t, operations{fetch: func(cfg dlg.Config, output string, report func(dlg.Progress)) (dlg.Result, error) {
		if cfg.Transcript == nil {
			t.Fatal("no transcript writer")
		}
		fmt.Fprintln(cfg.Transcript, ">> user")
		fmt.Fprintln(cfg.Transcript, "<< # ")
		return dlg.Result{Credential: dlg.Credential{User: "superadmin", Password: "s3cret"}, Path: output}, nil
	}})
	a.verbose = true
	tt := ui.NewTester(a.view, 960, 700)

	if err := tt.Click("获取超级管理员密码"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(a.logText, ">> user") {
		t.Fatalf("the transcript is not in the log: %q", a.logText)
	}
	if err := tt.Click("清空"); err != nil {
		t.Fatal(err)
	}
	if a.logText != "" {
		t.Fatalf("log after clearing = %q", a.logText)
	}
}

func TestLogKeepsTheEnd(t *testing.T) {
	a := newTestApp(t, operations{})
	for i := 0; i < maxLogLines*2; i++ {
		a.appendLog(fmt.Sprintf("line %d", i))
	}
	if n := strings.Count(a.logText, "\n") + 1; n > maxLogLines {
		t.Fatalf("log has %d lines, want at most %d", n, maxLogLines)
	}
	if !strings.HasSuffix(a.logText, fmt.Sprintf("line %d", maxLogLines*2-1)) {
		t.Fatalf("the log does not end with the newest line: %q", a.logText)
	}
}

// TestBusyStateLocksTheForm keeps a second run from starting while the first
// one is on the wire.
func TestBusyStateLocksTheForm(t *testing.T) {
	a := newTestApp(t, operations{fetch: func(dlg.Config, string, func(dlg.Progress)) (dlg.Result, error) {
		t.Error("a second run started while the first was busy")
		return dlg.Result{}, nil
	}})
	a.busy = true
	a.start()

	a.stage = "正在连接设备：192.168.1.1:23"
	tt := ui.NewTester(a.view, 960, 700)
	if !tt.HasText("正在运行…") {
		t.Errorf("the button does not show the run: %q", tt.Texts())
	}
	if !tt.HasText("正在连接设备：192.168.1.1:23") {
		t.Errorf("the stage is not shown: %q", tt.Texts())
	}
}

// TestTypingInTheForm edits what the window sends, and Enter runs it.
func TestTypingInTheForm(t *testing.T) {
	var got dlg.Config
	a := newTestApp(t, operations{fetch: func(cfg dlg.Config, output string, report func(dlg.Progress)) (dlg.Result, error) {
		got = cfg
		return dlg.Result{Path: output}, nil
	}})
	tt := ui.NewTester(a.view, 960, 700)

	// Tab reaches the tab bar first, then the first field of the form.
	tt.Key(0, ui.KeyTab)
	tt.Key(0, ui.KeyTab)
	if !tt.Focused("光猫地址") {
		t.Fatalf("the address field has no focus after two Tabs: %q", tt.Texts())
	}
	tt.Type("10.0.0.1")
	if a.host != "10.0.0.1" {
		t.Fatalf("host = %q after typing", a.host)
	}
	tt.Key(0, ui.KeyEnter)
	if got.Host != "10.0.0.1" {
		t.Fatalf("config host = %q, want what was typed", got.Host)
	}
}

func TestCmdEnterRunsThePage(t *testing.T) {
	runs := 0
	a := newTestApp(t, operations{fetch: func(cfg dlg.Config, output string, report func(dlg.Progress)) (dlg.Result, error) {
		runs++
		return dlg.Result{Path: output}, nil
	}})
	tt := ui.NewTester(a.view, 960, 700)

	tt.Key(ui.Cmd, ui.KeyEnter)
	if runs != 1 {
		t.Fatalf("runs = %d after Cmd+Enter", runs)
	}
}
