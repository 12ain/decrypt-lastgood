package main

import (
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/12ain/decrypt-lastgood/pkg/dlg"
)

// view builds the window, which MyGo draws itself: no HTML, no webview. It
// runs again after every input, so it reads the state and never keeps an
// element.
func (a *app) view(c *ui.Context) {
	t := c.Theme()
	// Cmd+Enter (Ctrl+Enter elsewhere) runs the page, wherever the focus is.
	c.OnShortcut(ui.Cmd, ui.KeyEnter, a.start)

	root := ui.Column(c).Fill()
	// A container dropped anywhere in the window is a file to decrypt.
	if files := root.DroppedFiles(); len(files) > 0 {
		a.takeDropped(files[0])
	}
	if root.FileDragOver() {
		root.Border(2, t.Accent)
	}
	root.Children(func() {
		a.header(c, t)
		a.pages(c, t)
		ui.Row(c).Grow(1).FillWidth().AlignItems(ui.Stretch).Children(func() {
			a.form(c, t)
			a.results(c, t)
		})
	})
}

func (a *app) header(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Padding(t.Space(5), t.Space(5), t.Space(3), t.Space(5)).Gap(t.Space(1)).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "光猫超级管理员密码").FontSize(19).Bold()
			ui.Spacer(c)
			ui.Textf(c, "dlg %s", version).FontSize(12).TextColor(t.TextMuted)
		})
		ui.Text(c, "用 telnet 取回 lastgood.xml，在本机解密，拿到超级管理员账号和密码。").
			FontSize(13).TextColor(t.TextMuted)
	})
}

func (a *app) pages(c *ui.Context, t *ui.Theme) {
	ui.Tabs(c, &a.tab, "获取凭据", "仅下载", "解密本地文件").
		Padding(t.Space(5), t.Space(5), 0, t.Space(5)).
		Disabled(a.busy)
	ui.Divider(c)
}

// form is the left pane: the parameters of the chosen page and its button.
func (a *app) form(c *ui.Context, t *ui.Theme) {
	ui.Scroll(c).Width(340).FillHeight().Children(func() {
		ui.Column(c).Padding(t.Space(5)).Gap(t.Space(3)).Children(func() {
			if a.tab == tabDecrypt {
				a.fileField(c, t, "加密文件", &a.input, "lastgood.xml.enc", true)
			} else {
				a.textField(c, t, "光猫地址", &a.host, "192.168.1.1")
				a.textField(c, t, "Telnet 用户名", &a.user, "机身标签上的普通账号")
				a.passwordField(c, t)
				a.portAndTimeout(c, t)
				a.textField(c, t, "远端文件路径", &a.remote, dlg.DefaultRemotePath)
			}

			if a.tab == tabDownload {
				a.fileField(c, t, "加密文件存到", &a.output, dlg.DefaultEncryptedOutput, false)
			} else {
				a.fileField(c, t, "配置文件存到", &a.output, dlg.DefaultOutput, false)
			}

			if a.tab != tabDecrypt {
				ui.Checkbox(c, &a.verbose, "记录 telnet 会话（排查用）").Disabled(a.busy)
			}

			a.actionButton(c)
			a.firstSteps(c, t)
		})
	})
}

func (a *app) textField(c *ui.Context, t *ui.Theme, label string, value *string, placeholder string) {
	ui.Column(c).Gap(t.Space(1)).Children(func() {
		ui.Text(c, label).FontSize(12).TextColor(t.TextMuted)
		if ui.TextInput(c, value).Label(label).Placeholder(placeholder).Disabled(a.busy).Submitted() {
			a.start()
		}
	})
}

func (a *app) passwordField(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Gap(t.Space(1)).Children(func() {
		ui.Text(c, "Telnet 密码").FontSize(12).TextColor(t.TextMuted)
		if ui.TextInput(c, &a.password).Label("Telnet 密码").
			Placeholder("机身标签上的密码").Password().Disabled(a.busy).Submitted() {
			a.start()
		}
	})
}

// portAndTimeout puts the two short numbers side by side.
func (a *app) portAndTimeout(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Gap(t.Space(3)).Children(func() {
		ui.Column(c).Gap(t.Space(1)).Width(90).Children(func() {
			ui.Text(c, "端口").FontSize(12).TextColor(t.TextMuted)
			ui.TextInput(c, &a.port).Label("端口").Placeholder("23").Disabled(a.busy)
		})
		ui.Column(c).Gap(t.Space(1)).Grow(1).Children(func() {
			ui.Text(c, "超时（秒）").FontSize(12).TextColor(t.TextMuted)
			ui.TextInput(c, &a.timeout).Label("超时（秒）").Placeholder("8").Disabled(a.busy)
		})
	})
}

// fileField is a path, with the system's picker beside it.
func (a *app) fileField(c *ui.Context, t *ui.Theme, label string, value *string, placeholder string, forOpen bool) {
	ui.Column(c).Gap(t.Space(1)).Children(func() {
		ui.Text(c, label).FontSize(12).TextColor(t.TextMuted)
		ui.Row(c).Gap(t.Space(1.5)).Children(func() {
			ui.TextInput(c, value).Label(label).Placeholder(placeholder).Disabled(a.busy).Grow(1)
			if ui.Button(c, "选择…").Disabled(a.busy).Clicked() {
				a.pickFile(value, forOpen)
			}
		})
	})
}

func (a *app) actionButton(c *ui.Context) {
	if a.busy {
		ui.Button(c, "正在运行…").FillWidth().Disabled(true)
		return
	}
	label := "获取超级管理员密码"
	switch a.tab {
	case tabDownload:
		label = "只下载加密文件"
	case tabDecrypt:
		label = "解密这个文件"
	}
	if ui.PrimaryButton(c, label).FillWidth().KeyShortcuts(ui.Cmd, ui.KeyEnter).Clicked() {
		a.start()
	}
}

// firstSteps is the one-time setup nobody reads the README for.
func (a *app) firstSteps(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Gap(t.Space(1)).Children(func() {
		ui.Text(c, "第一次用要先开 Telnet，出厂默认是关的。").FontSize(12).TextColor(t.TextMuted)
		if a.tab == tabDecrypt {
			return
		}
		host := strings.TrimSpace(a.host)
		if host == "" {
			return
		}
		ui.Link(c, dlg.TelnetPageURL(host), dlg.TelnetPageURL(host)).FontSize(12)
	})
}

// results is the right pane: the account, where the run stands, and the log.
func (a *app) results(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Grow(1).FillHeight().Padding(t.Space(5)).Gap(t.Space(3)).
		BorderStartWidth(1).BorderColor(t.Border).Children(func() {
		a.credentialCard(c, t)
		a.status(c, t)
		a.logSection(c, t)
	})
}

func (a *app) credentialCard(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Padding(t.Space(4)).Gap(t.Space(2)).Radius(10).
		Background(t.Surface).Border(1, t.Border).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "超级管理员").FontSize(12).TextColor(t.TextMuted)
			ui.Spacer(c)
			if !a.cred.Complete() {
				return
			}
			if ui.Button(c, "复制账号").Clicked() {
				c.Services().WriteClipboard(a.cred.User)
				c.Toast("账号已复制")
			}
			if ui.Button(c, "复制密码").Clicked() {
				c.Services().WriteClipboard(a.cred.Password)
				c.Toast("密码已复制")
			}
		})

		a.credRow(c, t, "账号", a.cred.User, false)
		a.credRow(c, t, "密码", a.shownPassword(), true)
	})
}

func (a *app) credRow(c *ui.Context, t *ui.Theme, label, value string, secret bool) {
	if value == "" {
		value = "—"
	}
	ui.Row(c).Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).Width(36).FontSize(12).TextColor(t.TextMuted)
		ui.Text(c, value).Grow(1).FontSize(16).Bold().Selectable()
		if secret && a.cred.Password != "" {
			toggle := "显示"
			if a.reveal {
				toggle = "隐藏"
			}
			if ui.Button(c, toggle).Clicked() {
				a.reveal = !a.reveal
			}
		}
	})
}

// shownPassword hides the password until the user asks for it: the window may
// well be on a shared screen.
func (a *app) shownPassword() string {
	if a.cred.Password == "" {
		return ""
	}
	if a.reveal {
		return a.cred.Password
	}
	return strings.Repeat("•", len([]rune(a.cred.Password)))
}

func (a *app) status(c *ui.Context, t *ui.Theme) {
	switch {
	case a.busy:
		ui.Row(c).Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
			ui.Spinner(c)
			ui.Text(c, a.stage).FontSize(13)
			ui.Spacer(c)
		})
		ui.Progress(c, a.progress)

	case a.errText != "":
		ui.Column(c).Padding(t.Space(3)).Gap(t.Space(2)).Radius(8).
			Background(t.Surface).Border(1, t.Danger).Children(func() {
			ui.Text(c, a.errText).FontSize(13).TextColor(t.Danger)
			if a.hintURL != "" {
				ui.Link(c, "打开开启 Telnet 的页面", a.hintURL).FontSize(13)
			}
		})

	case a.savedPath != "":
		ui.Column(c).Gap(t.Space(1.5)).Children(func() {
			ui.Textf(c, "已保存：%s", a.savedPath).FontSize(12).TextColor(t.TextMuted)
			ui.Row(c).Gap(t.Space(1.5)).Children(func() {
				if ui.Button(c, "在文件夹中显示").Clicked() {
					mygo.Shell.ShowItemInFolder(a.savedPath)
				}
				if ui.Button(c, "复制路径").Clicked() {
					c.Services().WriteClipboard(a.savedPath)
					c.Toast("路径已复制")
				}
			})
		})

	default:
		ui.Text(c, "填好左边的参数，点一下按钮。对光猫只跑只读命令，不改它的配置。").
			FontSize(13).TextColor(t.TextMuted)
	}
}

func (a *app) logSection(c *ui.Context, t *ui.Theme) {
	ui.Row(c).AlignItems(ui.Center).Gap(t.Space(1)).Children(func() {
		ui.Text(c, "运行日志").FontSize(12).Bold().TextColor(t.TextMuted)
		ui.Spacer(c)
		if ui.Button(c, "复制日志").Disabled(a.logText == "").Clicked() {
			c.Services().WriteClipboard(a.logText)
			c.Toast("日志已复制")
		}
		if ui.Button(c, "清空").Disabled(a.logText == "" || a.busy).Clicked() {
			a.logText = ""
		}
	})
	ui.TextArea(c, &a.logText).ReadOnly(true).Grow(1).MinHeight(t.Space(28)).FontSize(12).
		Placeholder("连接、下载、解密的每一步都记在这里；勾上「记录 telnet 会话」还能看到原始收发。")
}
