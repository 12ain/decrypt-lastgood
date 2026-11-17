package main

import (
	"image/png"
	"os"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/12ain/decrypt-lastgood/pkg/dlg"
)

// TestSnapshot writes what the window looks like, for a human to look at:
//
//	GUI_SNAPSHOT=/tmp/dlg.png go test ./gui -run TestSnapshot
//
// It renders without a window, so it works on a build machine too.
func TestSnapshot(t *testing.T) {
	out := os.Getenv("GUI_SNAPSHOT")
	if out == "" {
		t.Skip("GUI_SNAPSHOT not set")
	}

	states := map[string]func(*app){
		"empty": func(a *app) {},
		"result": func(a *app) {
			a.cred = dlg.Credential{User: "superadmin", Password: "example-password"}
			a.savedPath = "/Users/you/lastgood.xml"
			a.stage = "完成"
			a.progress = 1
			a.logText = "14:22:01  正在连接设备：192.168.1.1:23\n" +
				"14:22:05  正在取回配置文件：/config/work/lastgood.xml\n" +
				"14:22:06  已取得超级管理员账号\n" +
				"14:22:06  完成，用时 5.2s"
		},
		"error": func(a *app) {
			a.busy = true
			a.stage = "正在连接设备：192.168.1.1:23"
			a.progress = -1
		},
		"hint": func(a *app) {
			a.hintURL = dlg.TelnetPageURL("192.168.1.1")
			a.errText = "连不上设备的 telnet 端口。光猫出厂默认关着它，先把开关打开再试。"
			a.logText = "14:22:01  正在连接设备：192.168.1.1:23\n14:22:09  失败：连接超时"
		},
	}

	for name, prepare := range states {
		a := newTestApp(t, operations{})
		prepare(a)
		img := ui.Render(a.view, 960, 700, 2)
		path := out + "-" + name + ".png"
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
		f.Close()
		t.Log("wrote " + path)

		dark := ui.NewTesterWith(a.view, 960, 700, ui.TesterOptions{Dark: true})
		df, err := os.Create(out + "-" + name + "-dark.png")
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(df, dark.Image()); err != nil {
			t.Fatal(err)
		}
		df.Close()
	}
}
