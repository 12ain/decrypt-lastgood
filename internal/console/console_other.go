//go:build !windows

package console

import (
	"bufio"
	"errors"
	"os"
	"os/exec"
	"strings"
)

// ReadLine reads one line from stdin without echoing it.
func ReadLine() (string, error) {
	restore := disableEcho()
	defer restore()

	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", errors.New("could not read password from stdin")
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// disableEcho turns off terminal echo via stty and returns a restore function.
// When stdin is not a terminal stty fails and this becomes a no-op.
func disableEcho() func() {
	off := exec.Command("stty", "-echo")
	off.Stdin = os.Stdin
	if err := off.Run(); err != nil {
		return func() {}
	}

	return func() {
		on := exec.Command("stty", "echo")
		on.Stdin = os.Stdin
		_ = on.Run()
	}
}
