package telnet

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeShell is a minimal BusyBox-like telnet server used to exercise the client
// against a device that echoes every byte typed at it, which is what a real tty
// in canonical mode does.
type fakeShell struct {
	listener net.Listener
	file     []byte
	path     string

	// delay is inserted between echoing a command line and running it, which
	// models a slow modem and forces the client to cope with the echo and the
	// output arriving in separate reads.
	delay time.Duration
	// noBase64 makes the base64 probes fail.
	noBase64 bool
	// prompt is what the shell prints before each command, e.g. "# " or "~ $ ".
	prompt string

	wg sync.WaitGroup
}

func newFakeShell(t *testing.T, file []byte, path string, delay time.Duration, noBase64 bool) *fakeShell {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	sh := &fakeShell{listener: listener, file: file, path: path, delay: delay, noBase64: noBase64, prompt: "# "}
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
	t.Cleanup(func() {
		listener.Close()
		sh.wg.Wait()
	})
	return sh
}

func (s *fakeShell) target() Target {
	return Target{
		Host:    "127.0.0.1",
		Port:    s.listener.Addr().(*net.TCPAddr).Port,
		User:    "user",
		Pass:    "secret",
		Timeout: 2 * time.Second,
	}
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
	write("\r\n" + s.prompt)

	var pending []byte
	buffer := make([]byte, 1024)
	for {
		n, err := conn.Read(buffer)
		if n > 0 {
			// tty echo, before the shell gets a chance to run anything
			conn.Write(buffer[:n])
			pending = append(pending, buffer[:n]...)

			for {
				i := bytes.IndexByte(pending, '\n')
				if i < 0 {
					break
				}
				line := strings.TrimRight(string(pending[:i]), "\r")
				pending = pending[i+1:]
				if line != "" {
					time.Sleep(s.delay)
					write(s.exec(line))
					write(s.prompt)
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
		// Models "echo aGk= | base64 -d", the probe used to confirm that the
		// device can actually decode base64.
		if strings.Contains(line, "|") && !s.noBase64 {
			if decoded, err := base64.StdEncoding.DecodeString(fields[1]); err == nil {
				// base64 -d writes the decoded bytes as they are, so "aGk="
				// comes back as "hi" with no trailing newline and the prompt
				// ends up glued to it.
				return string(decoded)
			}
		}
		return strings.Join(fields[1:], " ") + "\r\n"
	case "wc":
		return fmt.Sprintf("%d %s\r\n", len(s.file), s.path)
	case "base64", "busybox":
		if s.noBase64 {
			return fields[0] + ": not found\r\n"
		}
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

// TestRunSurvivesCommandEcho is the regression test for the original bug: the
// device echoes the typed lines immediately, so the echo of
// "echo __DLG_END_n__" reaches the client before the command has produced any
// output. The read must not stop there.
func TestRunSurvivesCommandEcho(t *testing.T) {
	sh := newFakeShell(t, nil, "/config/work/lastgood.xml", 200*time.Millisecond, false)

	client, err := Dial(sh.target())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	output, err := client.Run("echo hello", 5*time.Second)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if output != "hello" {
		t.Fatalf("output = %q, want %q", output, "hello")
	}
}

func TestFetchFileOverEchoingShell(t *testing.T) {
	// Deliberately larger than one base64 line to cover chunked payloads.
	payload := bytes.Repeat([]byte("lastgood-config-0123456789"), 200)
	sh := newFakeShell(t, payload, "/config/work/lastgood.xml", 250*time.Millisecond, false)

	client, err := Dial(sh.target())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	data, err := client.FetchFile("/config/work/lastgood.xml")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if !bytes.Equal(data, payload) {
		t.Fatalf("payload mismatch: got %d bytes, want %d", len(data), len(payload))
	}
}

// TestFetchFileWhenProbeHasNoNewline covers the device that triggered this
// round of fixes: its prompt is "~ $ " and "base64 -d" prints the decoded bytes
// without a newline, so the probe output arrives glued to the prompt
// ("hi~ $ echo __DLG_END_2__") and used to be stripped away entirely.
func TestFetchFileWhenProbeHasNoNewline(t *testing.T) {
	payload := []byte("<xml>aucTeleAccountName>CMCCAdmin</xml>")
	sh := newFakeShell(t, payload, "/config/work/lastgood.xml", 0, false)
	sh.prompt = "~ $ "

	client, err := Dial(sh.target())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	data, err := client.FetchFile("/config/work/lastgood.xml")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if !bytes.Equal(data, payload) {
		t.Fatalf("payload mismatch: %q", data)
	}
}

func TestFetchFileReportsMissingBase64(t *testing.T) {
	sh := newFakeShell(t, []byte("x"), "/config/work/lastgood.xml", 0, true)

	client, err := Dial(sh.target())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	_, err = client.FetchFile("/config/work/lastgood.xml")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "base64") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMarkerPatternIgnoresEchoedCommand(t *testing.T) {
	pattern := markerPattern("__DLG_END_1__")

	cases := []struct {
		text string
		want bool
	}{
		{"__DLG_END_1__\r\n", true},
		{"# __DLG_END_1__\r\n", true},
		{"~ $ __DLG_END_1__\r\n", true},
		{"echo __DLG_END_1__\r\n", false},
		{"# echo __DLG_END_1__\r\n", false},
		{"__DLG_END_1__ is not a marker here\r\n", false},
	}

	for _, tc := range cases {
		if got := pattern.MatchString(tc.text); got != tc.want {
			t.Fatalf("match(%q) = %v, want %v", tc.text, got, tc.want)
		}
	}
}
