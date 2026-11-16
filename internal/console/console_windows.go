//go:build windows

// Package console reads a password from the terminal without echoing it.
package console

import (
	"bufio"
	"errors"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

const (
	stdInputHandle       = 0xFFFFFFF6 // (DWORD)-10
	enableEchoInput      = 0x0004
	enableLineInput      = 0x0002
	enableProcessedInput = 0x0001
)

var (
	kernel32      = syscall.NewLazyDLL("kernel32.dll")
	procGetHandle = kernel32.NewProc("GetStdHandle")
	procGetMode   = kernel32.NewProc("GetConsoleMode")
	procSetMode   = kernel32.NewProc("SetConsoleMode")
)

// ReadLine reads one line from stdin without echoing it.
func ReadLine() (string, error) {
	handle, _, _ := procGetHandle.Call(stdInputHandle)

	var mode uint32
	if ok, _, _ := procGetMode.Call(handle, uintptr(unsafe.Pointer(&mode))); ok != 0 {
		// Keep line input enabled so the console still buffers a whole line,
		// but drop the echo so the password is not shown.
		if r, _, _ := procSetMode.Call(handle, uintptr(mode&^enableEchoInput)); r != 0 {
			defer procSetMode.Call(handle, uintptr(mode))
		}
	}

	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", errors.New("could not read password from stdin")
	}
	return strings.TrimRight(line, "\r\n"), nil
}
