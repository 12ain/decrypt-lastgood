// Command dlg-gui is the desktop client of dlg: the same fetch, decrypt and
// extract steps as the command line tool, in a window that MyGo draws itself,
// with no webview and no frontend to build.
package main

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/12ain/decrypt-lastgood/pkg/dlg"
)

// version is what the header shows: the same one the command line reports.
const version = dlg.Version

func main() {
	a := newApp()

	mygo.App.SetName("dlg")
	mygo.App.WhenReady(func() {
		// Native UI: the window shows a Go view drawn on the GPU, not a page.
		win := mygo.NewWindow(mygo.WindowOptions{
			Title:     "dlg — 光猫超级管理员密码",
			Width:     960,
			Height:    700,
			MinWidth:  780,
			MinHeight: 560,
			// Opens where the user left it last time.
			StateKey: "main",
			Content:  ui.View(a.view),
		})
		a.attach(win)
	})

	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// openFileDialog asks for an existing container.
func openFileDialog(current string) (string, error) {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Title:       "选择加密的配置文件",
		DefaultPath: current,
		Filters: []mygo.FileFilter{{
			Name:       "加密的配置文件",
			Extensions: []string{"enc", "xml"},
		}},
	})
	if err != nil || len(paths) == 0 {
		return "", err
	}
	return paths[0], nil
}

// saveFileDialog asks where the decrypted XML goes.
func saveFileDialog(current string) (string, error) {
	if current == "" {
		current = dlg.DefaultOutputPath()
	}
	return mygo.Dialog.Save(mygo.SaveDialogOptions{
		Title:       "保存解密后的配置文件",
		DefaultPath: current,
		Filters: []mygo.FileFilter{{
			Name:       "配置文件",
			Extensions: []string{"xml"},
		}},
	})
}
