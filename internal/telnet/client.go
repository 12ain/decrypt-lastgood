// Package telnet implements the small subset of the telnet protocol needed to
// drive the optical modem's BusyBox shell, replacing Python's removed telnetlib.
package telnet

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
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
}

// Client is an authenticated telnet session.
type Client struct {
	conn    net.Conn
	target  Target
	counter int

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

	client := &Client{conn: conn, target: target}
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
		if err := c.send(c.target.User); err != nil {
			return err
		}
		time.Sleep(600 * time.Millisecond)
		c.idle(2 * time.Second)
	}

	if err := c.send(c.target.Pass); err != nil {
		return err
	}
	time.Sleep(time.Second)

	output := bytes.ToLower(c.idle(3 * time.Second))
	if bytes.Contains(output, []byte("incorrect")) || bytes.Contains(output, []byte("failed")) {
		return errors.New("telnet login failed")
	}
	return nil
}

// Run executes a command and returns its cleaned output. A pair of unique
// markers is echoed around the command so that shell prompts and the echoes of
// the command itself can be stripped off reliably.
func (c *Client) Run(command string, timeout time.Duration) (string, error) {
	c.counter++
	start := fmt.Sprintf("__DLG_START_%d__", c.counter)
	end := fmt.Sprintf("__DLG_END_%d__", c.counter)

	for _, line := range []string{"echo " + start, command, "echo " + end} {
		if err := c.send(line); err != nil {
			return "", err
		}
	}

	var buffer []byte
	endMarker := []byte(end)
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if bytes.Contains(buffer, endMarker) {
			break
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
	if i := strings.Index(text, start); i >= 0 {
		text = text[i+len(start):]
	}
	if i := strings.Index(text, end); i >= 0 {
		text = text[:i]
	}
	return cleanOutput(text, command), nil
}

// FetchFile reads a remote binary file by base64-encoding it on the device.
// Only read-only commands (command -v, wc, base64) are sent.
func (c *Client) FetchFile(remotePath string) ([]byte, error) {
	if strings.TrimSpace(mustRun(c, "command -v base64", 8*time.Second)) == "" {
		return nil, errors.New("remote device does not provide base64")
	}

	quoted := shellQuote(remotePath)

	sizeOutput, err := c.Run("wc -c "+quoted, 8*time.Second)
	if err != nil {
		return nil, err
	}
	expected, err := parseWCSize(sizeOutput)
	if err != nil {
		return nil, err
	}

	encoded, err := c.Run("base64 "+quoted, 120*time.Second)
	if err != nil {
		return nil, err
	}

	var payload strings.Builder
	for _, line := range strings.Split(encoded, "\n") {
		line = strings.TrimSpace(line)
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

var (
	ansiRE       = regexp.MustCompile("\x1b\\[[0-9;]*[A-Za-z]")
	base64LineRE = regexp.MustCompile(`^[A-Za-z0-9+/=]+$`)
	digitsRE     = regexp.MustCompile(`^\d+$`)
)

func mustRun(c *Client, command string, timeout time.Duration) string {
	out, err := c.Run(command, timeout)
	if err != nil {
		return ""
	}
	return out
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

	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(ansiRE.ReplaceAllString(line, ""))
		switch {
		case line == "":
			continue
		case strings.Contains(line, "__DLG_START_"), strings.Contains(line, "__DLG_END_"):
			continue
		case line == command:
			continue
		case strings.HasPrefix(line, "echo __DLG_"):
			continue
		case strings.HasSuffix(line, " $"), strings.HasPrefix(line, "~ $"):
			continue
		}
		kept = append(kept, line)
	}

	return strings.Join(kept, "\n")
}
