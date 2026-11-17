// Package dlg is what the command line tool and the desktop client do,
// without a front end: log in to the modem over telnet, bring the encrypted
// lastgood.xml back, decrypt it, and read the super administrator account
// out of the XML.
//
// Both front ends call these functions, so they take the same steps in the
// same order and report the same failures. The command line tool adds flag
// parsing and terminal output on top, the desktop client a window.
package dlg

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/12ain/decrypt-lastgood/internal/container"
	"github.com/12ain/decrypt-lastgood/internal/telnet"
)

// Version is the release both front ends report. Keep it in step with
// gui/mygo.json, which the packaged app carries.
const Version = "0.2.0"

// Defaults of a connection and of the files it writes.
const (
	DefaultPort            = 23
	DefaultTimeout         = 8 * time.Second
	DefaultRemotePath      = "/config/work/lastgood.xml"
	DefaultOutput          = "lastgood.xml"
	DefaultEncryptedOutput = "lastgood.xml.enc"
)

// TelnetPagePath is the page that turns telnet on in the CMCC firmware. Telnet
// is off by default, which is the first thing a new user trips over.
const TelnetPagePath = "/webcmcc/telnet_page.html"

// Config describes one device and the session to it.
type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	// RemotePath is the configuration file on the device.
	RemotePath string
	Timeout    time.Duration

	// Transcript, when set, receives a line per direction of the session:
	// what is sent, and the raw bytes received. This is the -v output of the
	// command line tool.
	Transcript io.Writer
}

// WithDefaults fills in the fields left zero, so that a caller only sets what
// the user changed.
func (c Config) WithDefaults() Config {
	if c.Port == 0 {
		c.Port = DefaultPort
	}
	if c.RemotePath == "" {
		c.RemotePath = DefaultRemotePath
	}
	if c.Timeout <= 0 {
		c.Timeout = DefaultTimeout
	}
	return c
}

// Validate reports whether the configuration can be used at all.
func (c Config) Validate() error {
	if strings.TrimSpace(c.Host) == "" {
		return errors.New("必须指定光猫地址")
	}
	if strings.TrimSpace(c.User) == "" {
		return errors.New("必须指定 telnet 用户名")
	}
	return nil
}

// Address is host:port, as a dialer and a log line write it.
func (c Config) Address() string {
	return net.JoinHostPort(c.Host, strconv.Itoa(c.WithDefaults().Port))
}

// TelnetPageURL is the page that turns telnet on for this device.
func (c Config) TelnetPageURL() string { return TelnetPageURL(c.Host) }

// TelnetPageURL builds the page that toggles telnet for the given device.
func TelnetPageURL(host string) string {
	return "http://" + host + TelnetPagePath
}

// Stage is the step a Progress reports. The front ends turn it into their own
// words; the library keeps them language-neutral.
type Stage int

// The steps of a fetch, in the order they happen.
const (
	StageConnect Stage = iota
	StageDownload
	StageDecrypt
	StageExtract
	StageSave
)

// Progress is one step of work, reported as it starts.
type Progress struct {
	Stage Stage
	// Detail is what the step works on: "192.168.1.1:23" while connecting,
	// the remote path while downloading, the local path while saving.
	Detail string
	// Fraction is how much of the step is done, from 0 to 1; negative when
	// its length is unknown.
	Fraction float64
}

// Fetch logs in to the device and returns the still-encrypted configuration
// file. It only sends read-only commands. report, when set, is called on the
// calling goroutine as each step starts, so it must not block for long.
func Fetch(cfg Config, report func(Progress)) ([]byte, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	cfg = cfg.WithDefaults()

	if report != nil {
		report(Progress{Stage: StageConnect, Detail: cfg.Address(), Fraction: -1})
	}

	client, err := dial(cfg)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	if report != nil {
		report(Progress{Stage: StageDownload, Detail: cfg.RemotePath, Fraction: -1})
	}
	return client.FetchFile(cfg.RemotePath)
}

// dial connects and authenticates, marking the failures that mean the device
// could not be reached at all.
func dial(cfg Config) (*telnet.Client, error) {
	client, err := telnet.Dial(telnet.Target{
		Host:    cfg.Host,
		Port:    cfg.Port,
		User:    cfg.User,
		Pass:    cfg.Password,
		Timeout: cfg.Timeout,
		Debug:   cfg.Transcript,
	})
	if err != nil && strings.Contains(err.Error(), "connect to") {
		return nil, fmt.Errorf("%w：%v", ErrConnect, err)
	}
	return client, err
}

// Decrypt is the full pipeline on bytes already on disk: container, then the
// tar.gz inside it, then the configuration XML.
func Decrypt(data []byte) ([]byte, error) {
	return container.DecryptXML(data)
}

// Credential is the super administrator account of the device.
type Credential struct {
	User     string
	Password string
}

// Complete reports whether both fields were found.
func (c Credential) Complete() bool {
	return c.User != "" && c.Password != ""
}

// ReadCredential scans the decrypted configuration for the super
// administrator account, in either of the two shapes the firmware writes.
func ReadCredential(configXML []byte) Credential {
	cred := container.FindCredential(configXML)
	return Credential{User: cred.User, Password: cred.Password}
}

// ReadFile reads a file the user picked.
func ReadFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// WriteFile writes a file with mode 0600: it holds the administrator password.
// Missing parent directories are created.
func WriteFile(path string, data []byte) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0o600)
}

// AbsolutePath is what a message shows for a path: absolute when the working
// directory is known.
func AbsolutePath(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

// DefaultOutputPath is where the decrypted XML goes when the user did not say:
// lastgood.xml beside the working directory.
func DefaultOutputPath() string {
	if wd, err := os.Getwd(); err == nil {
		return filepath.Join(wd, DefaultOutput)
	}
	return DefaultOutput
}

// ErrConnect marks a failure to reach the device, as opposed to reaching it
// and finding the credentials or the shell wrong. Only the former is worth
// the "turn telnet on" hint.
var ErrConnect = errors.New("连不上设备")

// IsConnectError reports whether err means the device could not be reached.
func IsConnectError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, ErrConnect) || strings.Contains(err.Error(), "connect to")
}

// Result is what one run produced, for a front end that shows it.
type Result struct {
	Credential Credential
	XML        []byte
	// Encrypted is the file as downloaded, kept so that a caller can save it
	// without fetching again.
	Encrypted []byte
	// Path is where the decrypted XML was written.
	Path string
}

// FetchAndDecrypt runs the whole default command: fetch, decrypt, extract the
// credential, write the XML to output, and return all of it.
func FetchAndDecrypt(cfg Config, output string, report func(Progress)) (Result, error) {
	if output == "" {
		output = DefaultOutputPath()
	}

	encrypted, err := Fetch(cfg, report)
	if err != nil {
		return Result{}, err
	}

	if report != nil {
		report(Progress{Stage: StageDecrypt, Fraction: -1})
	}
	plain, err := Decrypt(encrypted)
	if err != nil {
		return Result{}, err
	}

	if report != nil {
		report(Progress{Stage: StageSave, Detail: output, Fraction: -1})
	}
	if err := WriteFile(output, plain); err != nil {
		return Result{}, err
	}

	return Result{
		Credential: ReadCredential(plain),
		XML:        plain,
		Encrypted:  encrypted,
		Path:       AbsolutePath(output),
	}, nil
}

// DecryptFile decrypts a container the user already has, writing the XML to
// output, and returns what it holds.
func DecryptFile(input, output string, report func(Progress)) (Result, error) {
	if input == "" {
		return Result{}, errors.New("必须指定要解密的文件")
	}
	if output == "" {
		output = DefaultOutputPath()
	}

	if report != nil {
		report(Progress{Stage: StageDecrypt, Detail: input, Fraction: -1})
	}
	encrypted, err := ReadFile(input)
	if err != nil {
		return Result{}, err
	}
	plain, err := Decrypt(encrypted)
	if err != nil {
		return Result{}, err
	}

	if report != nil {
		report(Progress{Stage: StageSave, Detail: output, Fraction: -1})
	}
	if err := WriteFile(output, plain); err != nil {
		return Result{}, err
	}

	return Result{
		Credential: ReadCredential(plain),
		XML:        plain,
		Encrypted:  encrypted,
		Path:       AbsolutePath(output),
	}, nil
}

// SaveEncrypted writes the downloaded container, for the "download only" mode.
func SaveEncrypted(data []byte, output string) (string, error) {
	if output == "" {
		output = DefaultEncryptedOutput
	}
	if err := WriteFile(output, data); err != nil {
		return "", err
	}
	return AbsolutePath(output), nil
}

// DescribeStage is the message a front end shows while a step runs, without
// the detail.
func DescribeStage(stage Stage) string {
	switch stage {
	case StageConnect:
		return "正在连接设备"
	case StageDownload:
		return "正在取回配置文件"
	case StageDecrypt:
		return "正在解密"
	case StageExtract:
		return "正在解包配置"
	case StageSave:
		return "正在保存文件"
	}
	return fmt.Sprintf("stage %d", int(stage))
}
