package container

import (
	"html"
	"regexp"
	"strings"
)

var (
	teleAccountNameRE = regexp.MustCompile(`(?s)<aucTeleAccountName\b[^>]*>(.*?)</aucTeleAccountName>`)
	teleAccountPassRE = regexp.MustCompile(`(?s)<aucTeleAccountPassword\b[^>]*>(.*?)</aucTeleAccountPassword>`)
)

// Credential holds the super administrator account recovered from the config.
type Credential struct {
	User     string
	Password string
}

// Complete reports whether both fields were found.
func (c Credential) Complete() bool {
	return c.User != "" && c.Password != ""
}

// FindCredential scans the decrypted config XML for the super administrator
// account. The vendor stores it in aucTeleAccountName / aucTeleAccountPassword.
func FindCredential(configXML []byte) Credential {
	return Credential{
		User:     captureText(teleAccountNameRE, configXML),
		Password: captureText(teleAccountPassRE, configXML),
	}
}

func captureText(re *regexp.Regexp, data []byte) string {
	match := re.FindSubmatch(data)
	if len(match) < 2 {
		return ""
	}

	value := strings.TrimSpace(string(match[1]))
	if strings.HasPrefix(value, "<![CDATA[") && strings.HasSuffix(value, "]]>") {
		return strings.TrimSpace(value[len("<![CDATA[") : len(value)-len("]]>")])
	}
	return strings.TrimSpace(html.UnescapeString(value))
}
