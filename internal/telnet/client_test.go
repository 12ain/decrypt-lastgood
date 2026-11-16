package telnet

import (
	"strings"
	"testing"
	"time"
)

func TestCleanOutputStripsMarkersPromptAndEcho(t *testing.T) {
	raw := "\x1b[1;32m__DLG_START_1__\x1b[0m\r\n" +
		"wc -c '/config/work/lastgood.xml'\r\n" +
		"123456 /config/work/lastgood.xml\r\n" +
		"~ $ echo __DLG_END_1__\r\n" +
		"__DLG_END_1__\r\n"

	got := cleanOutput(raw, "wc -c '/config/work/lastgood.xml'")
	if got != "123456 /config/work/lastgood.xml" {
		t.Fatalf("unexpected output %q", got)
	}
}

func TestParseWCSize(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{"123456 /config/work/lastgood.xml", 123456},
		{"  42  /tmp/x", 42},
		{"wc: /nope: No such file or directory", 0},
	}

	for _, tc := range cases {
		got, err := parseWCSize(tc.input)
		if tc.want == 0 {
			if err == nil {
				t.Fatalf("expected an error for %q", tc.input)
			}
			continue
		}
		if err != nil {
			t.Fatalf("parseWCSize(%q): %v", tc.input, err)
		}
		if got != tc.want {
			t.Fatalf("parseWCSize(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestFilterDropsNegotiationAndAnswersIt(t *testing.T) {
	client := &Client{}

	// IAC DO ECHO, then "hi", then IAC WILL SGA.
	raw := []byte{iac, do, 1, 'h', 'i', iac, will, 3}
	data, replies := client.filter(raw)

	if string(data) != "hi" {
		t.Fatalf("data = %q, want %q", data, "hi")
	}
	want := []byte{iac, wont, 1, iac, dont, 3}
	if string(replies) != string(want) {
		t.Fatalf("replies = %v, want %v", replies, want)
	}
}

func TestFilterPassesEscapedIAC(t *testing.T) {
	client := &Client{}
	data, replies := client.filter([]byte{'a', iac, iac, 'b'})

	if len(replies) != 0 {
		t.Fatalf("unexpected replies: %v", replies)
	}
	if string(data) != string([]byte{'a', iac, 'b'}) {
		t.Fatalf("data = %v", data)
	}
}

func TestFilterSkipsSubNegotiation(t *testing.T) {
	client := &Client{}
	// IAC SB TERMINAL-TYPE 1 IAC SE, then 'ok'
	raw := []byte{iac, sb, 24, 1, iac, se, 'o', 'k'}
	data, _ := client.filter(raw)

	if string(data) != "ok" {
		t.Fatalf("data = %q, want %q", data, "ok")
	}
}

func TestFilterHandlesSplitSequences(t *testing.T) {
	client := &Client{}

	data, _ := client.filter([]byte{iac, do})
	if len(data) != 0 {
		t.Fatalf("unexpected data: %v", data)
	}
	data, replies := client.filter([]byte{31, 'z'})
	if string(data) != "z" {
		t.Fatalf("data = %q, want %q", data, "z")
	}
	if string(replies) != string([]byte{iac, wont, 31}) {
		t.Fatalf("replies = %v", replies)
	}
}

func TestShellQuote(t *testing.T) {
	if got := shellQuote("/config/work/lastgood.xml"); got != "'/config/work/lastgood.xml'" {
		t.Fatalf("got %q", got)
	}
	if got := shellQuote("/tmp/a b"); got != "'/tmp/a b'" {
		t.Fatalf("got %q", got)
	}
}

func TestDialRejectsUnreachableHost(t *testing.T) {
	_, err := Dial(Target{
		Host:    "127.0.0.1",
		Port:    1,
		User:    "user",
		Pass:    "pass",
		Timeout: 500 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected a connection error")
	}
	if !strings.Contains(err.Error(), "connect to") {
		t.Fatalf("unexpected error: %v", err)
	}
}
