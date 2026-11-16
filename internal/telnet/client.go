// Package telnet implements the small subset of the telnet protocol needed to
// drive the optical modem's BusyBox shell, replacing Python's removed telnetlib.
package telnet

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Telnet control bytes (RFC 854).
const (
	iac  = 255
	dont = 254
	do   = 253
	wont = 252
	will = 251
	sb   = 250
	se   = 240
)

const (
	pollInterval = 200 * time.Millisecond
	readBuffer   = 8192
)

// Target describes where and how to connect.
type Target struct {
	Host    string
	Port    int
	User    string
	Pass    string
	Timeout time.Duration

	// Debug, when non-nil, receives a transcript of the session.
	Debug io.Writer
}

// Client is an authenticated telnet session.
type Client struct {
	conn    net.Conn
	target  Target
	counter int

	// debug, when set, receives a transcript of the session (both directions).
	debug io.Writer

	// Telnet negotiation state machine (see filter).
	state int
	verb  byte
}

// Dial connects and authenticates. The caller must Close the result.
func Dial(target Target) (*Client, error) {
	if target.Timeout <= 0 {
		target.Timeout = 8 * time.Second
	}

	address := net.JoinHostPort(target.Host, strconv.Itoa(target.Port))
	conn, err := net.DialTimeout("tcp", address, target.Timeout)
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", address, err)
	}

	client := &Client{conn: conn, target: target, debug: target.Debug}
	if err := client.login(); err != nil {
		conn.Close()
		return nil, err
	}
	return client, nil
}

// Close ends the session.
func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}
	_ = c.send("exit")
	return c.conn.Close()
}

func (c *Client) login() error {
	banner := bytes.ToLower(c.idle(3 * time.Second))
	if bytes.Contains(banner, []byte("login")) || bytes.Contains(banner, []byte("username")) {
		c.trace(">> " + c.target.User)
		if err := c.send(c.target.User); err != nil {
			return err
		}
		time.Sleep(600 * time.Millisecond)
		c.idle(2 * time.Second)
	}

	c.trace(">> " + strings.Repeat("*", len(c.target.Pass)))
	if err := c.send(c.target.Pass); err != nil {
		return err
	}
	time.Sleep(time.Second)

	output := bytes.ToLower(c.idle(3 * time.Second))
	if bytes.Contains(output, []byte("incorrect")) || bytes.Contains(output, []byte("failed")) {
		return errors.New("telnet login failed")
	}

	return c.verifyShell()
}

// verifyShell checks that the session really reached a shell prompt. Without
// this the next step fails with a puzzling "no base64" on devices where the
// login did not go through or telnet is a limited CLI.
func (c *Client) verifyShell() error {
	const marker = "__DLG_SHELL_OK__"

	hint := "check the password, and that telnet is enabled on the device (run with -v to see the raw session)"

	output, err := c.Run("echo "+marker, 5*time.Second)
	if err != nil {
		return fmt.Errorf("telnet login did not reach a shell (%w): %s", err, hint)
	}
	if !strings.Contains(output, marker) {
		return fmt.Errorf("telnet login did not reach a shell (no echo of %s): %s", marker, hint)
	}
	return nil
}

// Run executes a command and returns its cleaned output. A pair of unique
// markers is echoed around the command so that shell prompts and the echoes of
// the command itself can be stripped off reliably.
//
// The closing marker has to match a whole line: the device echoes the typed
// command back, and that echo ("echo __DLG_END_n__") contains the marker text,
// so a plain substring search would finish the read before any output arrived.
func (c *Client) Run(command string, timeout time.Duration) (string, error) {
	c.counter++
	start := fmt.Sprintf("__DLG_START_%d__", c.counter)
	end := fmt.Sprintf("__DLG_END_%d__", c.counter)

	for _, line := range []string{"echo " + start, command, "echo " + end} {
		c.trace(">> " + line)
		if err := c.send(line); err != nil {
			return "", err
		}
	}

	startRE := markerPattern(start)
	endRE := markerPattern(end)

	var buffer []byte
	deadline := time.Now().Add(timeout)

	for {
		if loc := endRE.FindIndex(buffer); loc != nil {
			// The match may include the tail of the output line and a prompt,
			// so cut at the marker itself.
			start := loc[0]
			if offset := bytes.Index(buffer[loc[0]:loc[1]], []byte(end)); offset >= 0 {
				start += offset
			}
			buffer = buffer[:start]
			break
		}
		if !time.Now().Before(deadline) {
			return "", fmt.Errorf("timed out after %s waiting for the output of %q", timeout, command)
		}

		c.conn.SetReadDeadline(time.Now().Add(pollInterval))

		chunk := make([]byte, readBuffer)
		n, err := c.conn.Read(chunk)
		if n > 0 {
			data, replies := c.filter(chunk[:n])
			if len(replies) > 0 {
				if _, werr := c.conn.Write(replies); werr != nil {
					return "", fmt.Errorf("answer telnet negotiation: %w", werr)
				}
			}
			c.trace("<< " + visible(data))
			buffer = append(buffer, data...)
		}
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			return "", errors.New("telnet connection closed")
		}
	}

	text := string(buffer)
	if loc := startRE.FindStringIndex(text); loc != nil {
		text = text[loc[1]:]
	} else if i := strings.Index(text, start); i >= 0 {
		// A device that does not echo commands still prints the marker itself.
		text = text[i+len(start):]
	}
	return cleanOutput(text, command), nil
}

// FetchFile reads a remote binary file by base64-encoding it on the device.
// Only read-only commands are sent.
func (c *Client) FetchFile(remotePath string) ([]byte, error) {
	encoder, err := c.resolveBase64()
	if err != nil {
		return nil, err
	}
	c.trace("using " + encoder)

	quoted := shellQuote(remotePath)

	sizeOutput, err := c.Run("wc -c "+quoted, 8*time.Second)
	if err != nil {
		return nil, fmt.Errorf("read remote size of %s: %w", remotePath, err)
	}
	expected, err := parseWCSize(sizeOutput)
	if err != nil {
		return nil, err
	}

	encoded, err := c.Run(encoder+" "+quoted, 120*time.Second)
	if err != nil {
		return nil, fmt.Errorf("read remote %s: %w", remotePath, err)
	}

	var payload strings.Builder
	for _, line := range strings.Split(encoded, "\n") {
		line = strings.TrimSpace(stripPrompt(line))
		if base64LineRE.MatchString(line) {
			payload.WriteString(line)
		}
	}
	if payload.Len() == 0 {
		return nil, fmt.Errorf("no base64 output returned for %s", remotePath)
	}

	data, err := base64.StdEncoding.DecodeString(payload.String())
	if err != nil {
		return nil, fmt.Errorf("decode base64 payload: %w", err)
	}
	if len(data) != expected {
		return nil, fmt.Errorf("downloaded size mismatch: got %d, expected %d", len(data), expected)
	}
	return data, nil
}

// resolveBase64 finds a working base64 implementation on the device. BusyBox
// sometimes ships the applet without a symlink in $PATH, hence the busier
// fallback.
func (c *Client) resolveBase64() (string, error) {
	candidates := []string{"base64", "busybox base64"}
	var failures []string

	for _, candidate := range candidates {
		out, err := c.Run("echo aGk= | "+candidate+" -d", 8*time.Second)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s (%v)", candidate, err))
			continue
		}
		if strings.TrimSpace(out) == "hi" {
			return candidate, nil
		}
		failures = append(failures, fmt.Sprintf("%s (%q)", candidate, strings.TrimSpace(out)))
	}

	return "", fmt.Errorf(
		"remote device does not provide a usable base64: tried %s; "+
			"check the firmware with -v to see the raw session",
		strings.Join(failures, ", "))
}

var (
	ansiRE       = regexp.MustCompile("\x1b\\[[0-9;]*[A-Za-z]")
	base64LineRE = regexp.MustCompile(`^[A-Za-z0-9+/=]+$`)
	digitsRE     = regexp.MustCompile(`^\d+$`)
	// promptClass matches a shell prompt in front of command output, e.g. "# ",
	// "~ $ " or "user@host:~ $ ". The optional prefix ends with a colon so that
	// output text glued to a prompt ("hi~ $ ") is not swallowed by the class.
	promptClass = `(?:[\w.@/\[\]-]{0,32}:)?(?:~[ \t])?[#$>][ \t]`
	// promptRE matches a shell prompt at the start of a line.
	promptRE = regexp.MustCompile(`^` + promptClass)
	// promptOnlyRE matches a line that is nothing but a left-over prompt.
	promptOnlyRE = regexp.MustCompile(`^[~#$> \t]+$`)
	// promptAtRE matches a prompt anywhere in the stream. It is used to put a
	// line break in front of a prompt that the device printed straight after
	// command output ("hi~ $ "), because it does not terminate the last output
	// line with a newline.
	promptAtRE = regexp.MustCompile(`[ \t]*~?[ \t]*[#$>]+[ \t]`)
)

// splitGluedPrompts puts a line break between command output and a prompt that
// the device printed without one, so that the prompt can be stripped without
// taking the tail of the output with it.
func splitGluedPrompts(text string) string {
	matches := promptAtRE.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return text
	}

	var out strings.Builder
	last := 0
	for _, match := range matches {
		start := match[0]
		for start < match[1] && (text[start] == ' ' || text[start] == '\t') {
			start++
		}
		// Prompts already at the start of a line need no help.
		if start == 0 || text[start-1] == '\n' {
			continue
		}
		out.WriteString(text[last:start])
		out.WriteString("\n")
		last = start
	}
	out.WriteString(text[last:])
	return out.String()
}

// markerPattern builds the pattern that recognises a marker printed by the
// device on a line of its own. A prompt may precede it, and so may the tail of
// an output line that did not end with a newline ("hi~ $ __DLG_END_2__"); the
// echoed command line ("echo __DLG_END_n__") still cannot match, because the
// marker has to follow the prompt character immediately.
func markerPattern(marker string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^(?:.*?[~#$>][ \t])?` + marker + `[ \t]*\r?$`)
}

// stripPrompt removes a leading shell prompt so that prompt-prefixed output
// lines still parse.
func stripPrompt(line string) string {
	if loc := promptRE.FindStringIndex(line); loc != nil {
		return line[loc[1]:]
	}
	return line
}

func parseWCSize(text string) (int, error) {
	for _, token := range strings.Fields(text) {
		if digitsRE.MatchString(token) {
			size, err := strconv.Atoi(token)
			if err != nil {
				break
			}
			return size, nil
		}
	}
	return 0, fmt.Errorf("could not parse remote file size from: %q", text)
}

// shellQuote wraps a path in single quotes, escaping embedded quotes.
func shellQuote(path string) string {
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}

func (c *Client) send(line string) error {
	_, err := c.conn.Write([]byte(line + "\r\n"))
	return err
}

// trace writes a line of the session transcript when debugging is enabled.
func (c *Client) trace(text string) {
	if c.debug == nil {
		return
	}
	fmt.Fprintln(c.debug, text)
}

// visible renders a received chunk readable: newlines stay, CR and other
// control bytes are escaped so they cannot scramble the transcript.
func visible(raw []byte) string {
	var sb strings.Builder
	for _, ch := range raw {
		switch {
		case ch == '\n':
			sb.WriteByte('\n')
		case ch == '\r':
			sb.WriteString(`\r`)
		case ch < 32 || ch > 126:
			fmt.Fprintf(&sb, "<%02x>", ch)
		default:
			sb.WriteByte(ch)
		}
	}
	return sb.String()
}

// idle reads until the line has been quiet for d.
func (c *Client) idle(d time.Duration) []byte {
	var data []byte
	last := time.Now()

	for {
		c.conn.SetReadDeadline(time.Now().Add(pollInterval))

		chunk := make([]byte, readBuffer)
		n, err := c.conn.Read(chunk)
		if n > 0 {
			clean, replies := c.filter(chunk[:n])
			if len(replies) > 0 {
				c.conn.Write(replies)
			}
			c.trace("<< " + visible(clean))
			data = append(data, clean...)
			last = time.Now()
		}
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				if time.Since(last) >= d {
					return data
				}
				continue
			}
			return data
		}
		if time.Since(last) >= d {
			return data
		}
	}
}

// filter strips telnet IAC sequences from raw bytes and returns the replies
// that must be sent back for option negotiation.
func (c *Client) filter(raw []byte) (data, replies []byte) {
	for _, b := range raw {
		switch c.state {
		case 0:
			if b == iac {
				c.state = 1
			} else {
				data = append(data, b)
			}
		case 1:
			switch b {
			case iac:
				data = append(data, iac)
				c.state = 0
			case do, dont, will, wont:
				c.verb = b
				c.state = 2
			case sb:
				c.state = 3
			default:
				c.state = 0
			}
		case 2:
			switch c.verb {
			case do:
				replies = append(replies, iac, wont, b)
			case will:
				replies = append(replies, iac, dont, b)
			}
			c.state = 0
		case 3:
			if b == iac {
				c.state = 4
			}
		case 4:
			if b == se {
				c.state = 0
			} else {
				c.state = 3
			}
		}
	}
	return data, replies
}

// cleanOutput mirrors the reference implementation: drop ANSI escapes, the
// marker lines, the echoed command and bare shell prompts.
func cleanOutput(text, command string) string {
	var kept []string

	for _, line := range strings.Split(splitGluedPrompts(text), "\n") {
		line = strings.TrimSpace(stripPrompt(ansiRE.ReplaceAllString(line, "")))
		switch {
		case line == "":
			continue
		case strings.Contains(line, "__DLG_START_"), strings.Contains(line, "__DLG_END_"):
			continue
		case line == command:
			continue
		case strings.HasPrefix(line, "echo __DLG_"):
			continue
		case strings.HasSuffix(line, " $"), strings.HasPrefix(line, "~ $"), promptOnlyRE.MatchString(line):
			continue
		}
		kept = append(kept, line)
	}

	return strings.Join(kept, "\n")
}
