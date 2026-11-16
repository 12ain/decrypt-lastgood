package container

import (
	"html"
	"regexp"
	"strings"
)

var (
	teleAccountNameRE = regexp.MustCompile(`(?s)<aucTeleAccountName\b[^>]*>(.*?)</aucTeleAccountName>`)
	teleAccountPassRE = regexp.MustCompile(`(?s)<aucTeleAccountPassword\b[^>]*>(.*?)</aucTeleAccountPassword>`)

	// Some firmwares write the config as <Value Name="..." Value="..."/> instead
	// of one element per setting, which is what lastgood.xml looks like on the
	// CMCC/Unionman builds.
	valueTagRE = regexp.MustCompile(`<Value\b[^>]*>`)
	xmlAttrRE  = regexp.MustCompile(`([A-Za-z_][\w.-]*)\s*=\s*(?:"([^"]*)"|'([^']*)')`)
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
// account. The vendor stores it in aucTeleAccountName / aucTeleAccountPassword,
// either as elements or as <Value Name="..." Value="..."/> entries.
func FindCredential(configXML []byte) Credential {
	return Credential{
		User:     firstNonEmpty(attributeValue(configXML, "aucTeleAccountName"), captureText(teleAccountNameRE, configXML)),
		Password: firstNonEmpty(attributeValue(configXML, "aucTeleAccountPassword"), captureText(teleAccountPassRE, configXML)),
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// attributeValue returns the Value attribute of the first <Value> tag whose Name
// attribute matches name.
func attributeValue(data []byte, name string) string {
	for _, tag := range valueTagRE.FindAll(data, -1) {
		attrs := parseAttributes(tag)
		if attrs["Name"] != name {
			continue
		}
		return html.UnescapeString(attrs["Value"])
	}
	return ""
}

func parseAttributes(tag []byte) map[string]string {
	attrs := make(map[string]string, 4)
	for _, match := range xmlAttrRE.FindAllSubmatch(tag, -1) {
		value := match[2]
		if len(value) == 0 {
			value = match[3]
		}
		attrs[string(match[1])] = strings.TrimSpace(html.UnescapeString(string(value)))
	}
	return attrs
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
