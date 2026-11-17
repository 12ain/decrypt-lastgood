package dlg_test

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/12ain/decrypt-lastgood/pkg/dlg"
)

func TestConfigDefaults(t *testing.T) {
	cfg := dlg.Config{Host: "192.168.1.1", User: "user"}.WithDefaults()
	if cfg.Port != dlg.DefaultPort || cfg.RemotePath != dlg.DefaultRemotePath || cfg.Timeout != dlg.DefaultTimeout {
		t.Fatalf("defaults = %+v", cfg)
	}
	if got := cfg.Address(); got != "192.168.1.1:23" {
		t.Fatalf("address = %q", got)
	}
	if got := cfg.TelnetPageURL(); got != "http://192.168.1.1/webcmcc/telnet_page.html" {
		t.Fatalf("telnet page = %q", got)
	}
}

func TestConfigKeepsWhatTheCallerSet(t *testing.T) {
	cfg := dlg.Config{Host: "10.0.0.1", User: "admin", Port: 2323, RemotePath: "/tmp/x.xml", Timeout: 3 * time.Second}
	got := cfg.WithDefaults()
	if got != cfg {
		t.Fatalf("WithDefaults changed a complete config: %+v", got)
	}
}

func TestConfigValidate(t *testing.T) {
	cases := map[string]dlg.Config{
		"no host": {User: "user"},
		"no user": {Host: "192.168.1.1"},
	}
	for name, cfg := range cases {
		if err := cfg.Validate(); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if err := (dlg.Config{Host: "192.168.1.1", User: "user"}).Validate(); err != nil {
		t.Fatalf("a complete config was rejected: %v", err)
	}
}

func TestReadCredentialElementForm(t *testing.T) {
	xml := `<InternetGatewayDevice><aucTeleAccountName>superadmin</aucTeleAccountName>` +
		`<aucTeleAccountPassword>p@ss</aucTeleAccountPassword></InternetGatewayDevice>`

	cred := dlg.ReadCredential([]byte(xml))
	if !cred.Complete() || cred.User != "superadmin" || cred.Password != "p@ss" {
		t.Fatalf("credential = %+v", cred)
	}
}

func TestReadCredentialValueForm(t *testing.T) {
	xml := `<Config><Value Name="aucTeleAccountName" Value="superadmin"/>` +
		`<Value Name="aucTeleAccountPassword" Value="p&amp;ss&lt;word"/></Config>`

	cred := dlg.ReadCredential([]byte(xml))
	if !cred.Complete() || cred.Password != "p&ss<word" {
		t.Fatalf("credential = %+v", cred)
	}
}

func TestReadCredentialMissing(t *testing.T) {
	if dlg.ReadCredential([]byte("<Config/>")).Complete() {
		t.Fatal("expected an incomplete credential")
	}
}

func TestDecryptRejectsGarbage(t *testing.T) {
	if _, err := dlg.Decrypt([]byte("not a container")); err == nil {
		t.Fatal("expected an error")
	}
}

func TestWriteFileIsPrivateAndCreatesDirectories(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "lastgood.xml")
	if err := dlg.WriteFile(path, []byte("<Config/>")); err != nil {
		t.Fatalf("write: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	// Windows has no Unix permission bits: the mode there says little about who
	// can read the file, so only the Unix builds check it.
	if runtime.GOOS != "windows" {
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Fatalf("mode = %o, want 600: the file holds the administrator password", perm)
		}
	}
}

func TestSaveEncryptedDefaultsTheName(t *testing.T) {
	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { os.Chdir(wd) })

	path, err := dlg.SaveEncrypted([]byte("body"), "")
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if filepath.Base(path) != dlg.DefaultEncryptedOutput {
		t.Fatalf("path = %q", path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat: %v", err)
	}
}

func TestIsConnectError(t *testing.T) {
	if dlg.IsConnectError(nil) {
		t.Fatal("nil is not a connection error")
	}
	if !dlg.IsConnectError(dlg.ErrConnect) {
		t.Fatal("ErrConnect must classify as a connection error")
	}
	// Port 1 is closed, so this is the "cannot reach the device" path
	// without touching a real modem.
	_, err := dlg.Fetch(dlg.Config{Host: "127.0.0.1", Port: 1, User: "user", Password: "x", Timeout: time.Second}, nil)
	if !dlg.IsConnectError(err) {
		t.Fatalf("expected a connection error, got %v", err)
	}
}

// TestFetchReadsTheFileOverTelnet is the library's end-to-end path: log in to a
// device that echoes what is typed at it, and bring the file back byte for
// byte. Decryption is a separate step (internal/container covers it), so the
// payload here is deliberately not a container.
func TestFetchReadsTheFileOverTelnet(t *testing.T) {
	payload := bytes.Repeat([]byte("0123456789abcdef"), 128)
	sh := newFakeShell(t, payload, "/config/work/lastgood.xml")
	defer sh.close()

	var stages []dlg.Stage
	cfg := dlg.Config{
		Host:     "127.0.0.1",
		Port:     sh.port(),
		User:     "user",
		Password: "secret",
		Timeout:  5 * time.Second,
	}

	var transcript bytes.Buffer
	cfg.Transcript = &transcript

	data, err := dlg.Fetch(cfg, func(p dlg.Progress) { stages = append(stages, p.Stage) })
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if !bytes.Equal(data, payload) {
		t.Fatalf("payload mismatch: got %d bytes, want %d", len(data), len(payload))
	}
	if len(stages) != 2 || stages[0] != dlg.StageConnect || stages[1] != dlg.StageDownload {
		t.Fatalf("stages = %v", stages)
	}
	if !strings.Contains(transcript.String(), ">> user") {
		t.Fatalf("transcript does not hold the login: %q", transcript.String())
	}
	if _, err := dlg.Decrypt(data); err == nil {
		t.Fatal("a non-container payload must not decrypt")
	}
}

func TestFetchRejectsAnIncompleteConfig(t *testing.T) {
	if _, err := dlg.Fetch(dlg.Config{Host: "192.168.1.1"}, nil); err == nil {
		t.Fatal("expected an error without a user name")
	}
}

func TestDecryptFileRoundTripNeedsAContainer(t *testing.T) {
	input := filepath.Join(t.TempDir(), "bad.enc")
	if err := os.WriteFile(input, []byte("definitely not a container"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	var stage dlg.Stage
	_, err := dlg.DecryptFile(input, "", func(p dlg.Progress) { stage = p.Stage })
	if err == nil {
		t.Fatal("expected an error")
	}
	if stage != dlg.StageDecrypt {
		t.Fatalf("stage = %v, want the decrypt step first", stage)
	}
	if _, err := dlg.DecryptFile("", "", nil); err == nil {
		t.Fatal("expected an error without an input file")
	}
}

func TestDescribeStageCoversEveryStage(t *testing.T) {
	stages := []dlg.Stage{dlg.StageConnect, dlg.StageDownload, dlg.StageDecrypt, dlg.StageExtract, dlg.StageSave}
	for _, stage := range stages {
		if got := dlg.DescribeStage(stage); got == "" || strings.HasPrefix(got, "stage ") {
			t.Errorf("stage %d has no description: %q", stage, got)
		}
	}
}

// fakeShell is a BusyBox-like telnet server: it asks for a login, echoes what
// is typed at it, answers the read-only commands the client sends, and hands
// file back as base64.
type fakeShell struct {
	listener net.Listener
	file     []byte
	path     string

	wg sync.WaitGroup
}

func newFakeShell(t *testing.T, file []byte, path string) *fakeShell {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	sh := &fakeShell{listener: listener, file: file, path: path}
	sh.wg.Add(1)
	go func() {
		defer sh.wg.Done()
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		sh.serve(conn)
	}()
	return sh
}

func (s *fakeShell) port() int { return s.listener.Addr().(*net.TCPAddr).Port }

func (s *fakeShell) close() {
	s.listener.Close()
	s.wg.Wait()
}

func (s *fakeShell) serve(conn net.Conn) {
	write := func(text string) { conn.Write([]byte(text)) }

	write("Login: ")
	if !readLine(conn) {
		return
	}
	write("Password: ")
	if !readLine(conn) {
		return
	}
	write("\r\n# ")

	var pending []byte
	buffer := make([]byte, 1024)
	for {
		n, err := conn.Read(buffer)
		if n > 0 {
			conn.Write(buffer[:n]) // tty echo
			pending = append(pending, buffer[:n]...)

			for {
				i := bytes.IndexByte(pending, '\n')
				if i < 0 {
					break
				}
				line := strings.TrimRight(string(pending[:i]), "\r")
				pending = pending[i+1:]
				if line != "" {
					write(s.exec(line))
					write("# ")
				}
			}
		}
		if err != nil {
			return
		}
	}
}

func (s *fakeShell) exec(line string) string {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return ""
	}

	switch fields[0] {
	case "echo":
		if strings.Contains(line, "|") {
			if decoded, err := base64.StdEncoding.DecodeString(fields[1]); err == nil {
				return string(decoded)
			}
		}
		return strings.Join(fields[1:], " ") + "\r\n"
	case "wc":
		return fmt.Sprintf("%d %s\r\n", len(s.file), s.path)
	case "base64", "busybox":
		encoded := base64.StdEncoding.EncodeToString(s.file)
		var out strings.Builder
		for i := 0; i < len(encoded); i += 76 {
			end := i + 76
			if end > len(encoded) {
				end = len(encoded)
			}
			out.WriteString(encoded[i:end] + "\r\n")
		}
		return out.String()
	}
	return "sh: " + fields[0] + ": not found\r\n"
}

func readLine(conn net.Conn) bool {
	buffer := make([]byte, 128)
	var line []byte
	for {
		n, err := conn.Read(buffer)
		if n > 0 {
			line = append(line, buffer[:n]...)
			if bytes.ContainsRune(line, '\n') {
				return true
			}
		}
		if err != nil {
			return false
		}
	}
}
