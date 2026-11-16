package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPartitionKeepsHostBeforeFlags(t *testing.T) {
	args := []string{"192.168.1.1", "-u", "user", "-p", "secret", "-P", "23", "-o", "out.enc"}

	positional, flags := partition(args, map[string]bool{
		"u": true, "p": true, "P": true, "r": true, "t": true, "o": true,
	})

	if len(positional) != 1 || positional[0] != "192.168.1.1" {
		t.Fatalf("positional = %v", positional)
	}
	want := []string{"-u", "user", "-p", "secret", "-P", "23", "-o", "out.enc"}
	if strings.Join(flags, " ") != strings.Join(want, " ") {
		t.Fatalf("flags = %v, want %v", flags, want)
	}
}

func TestPartitionHandlesEmptyPasswordValue(t *testing.T) {
	args := []string{"192.168.1.1", "-u", "user", "-p", ""}

	positional, flags := partition(args, map[string]bool{"u": true, "p": true})
	if len(positional) != 1 {
		t.Fatalf("positional = %v, expected only the host", positional)
	}
	if len(flags) != 4 {
		t.Fatalf("flags = %v", flags)
	}
}

func TestPartitionHandlesEqualsSyntax(t *testing.T) {
	args := []string{"192.168.1.1", "-u=user", "-P=2323"}

	positional, flags := partition(args, map[string]bool{"u": true, "P": true})
	if len(positional) != 1 {
		t.Fatalf("positional = %v", positional)
	}
	if len(flags) != 2 {
		t.Fatalf("flags = %v", flags)
	}
}

func run(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	code = Run(args, &out, &errBuf)
	return code, out.String(), errBuf.String()
}

func TestVersion(t *testing.T) {
	code, stdout, _ := run(t, "version")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(stdout, "dlg "+Version) {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestUnknownCommand(t *testing.T) {
	code, _, stderr := run(t, "nope")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "unknown command") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestDownloadRequiresUser(t *testing.T) {
	code, _, stderr := run(t, "download", "192.168.1.1")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "-u") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestDecryptMissingFile(t *testing.T) {
	code, _, stderr := run(t, "decrypt", filepath.Join(t.TempDir(), "absent.enc"))
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "error:") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestDecryptRejectsGarbage(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "bad.enc")
	if err := os.WriteFile(input, []byte("definitely not a container"), 0o600); err != nil {
		t.Fatalf("write input: %v", err)
	}

	code, _, stderr := run(t, "decrypt", input)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "too short") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestHelp(t *testing.T) {
	code, stdout, _ := run(t, "help")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(stdout, "dlg download") {
		t.Fatalf("stdout = %q", stdout)
	}
}
