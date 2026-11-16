package container

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/12ain/decrypt-lastgood/internal/kdf"
)

const sampleXML = `<?xml version="1.0" encoding="UTF-8"?>
<InternetGatewayDevice>
  <aucTeleAccountName>CMCCAdmin</aucTeleAccountName>
  <aucTeleAccountPassword>aDm8H%MdA</aucTeleAccountPassword>
  <X_CMCC_UserAccount>user</X_CMCC_UserAccount>
</InternetGatewayDevice>
`

// buildContainer produces a container laid out exactly like the vendor's, so the
// decryptor can be verified end to end without a real device dump.
func buildContainer(t *testing.T, payload []byte) []byte {
	t.Helper()

	c1 := make([]byte, componentSize)
	c2 := make([]byte, componentSize)
	c3 := make([]byte, componentSize)
	salt := make([]byte, componentSize)
	for _, buf := range [][]byte{c1, c2, c3, salt} {
		if _, err := rand.Read(buf); err != nil {
			t.Fatalf("rand: %v", err)
		}
	}

	secret := make([]byte, componentSize)
	for i := range secret {
		secret[i] = c1[i] ^ c2[i] ^ c3[i]
	}

	wrap2Key, err := kdf.SHA256(secret, salt, pbkdf2Rounds, wrapKeyLen)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}

	wrap1Key := randomBytes(t, wrapKeyLen)
	fileKey := randomBytes(t, fileKeyLen)

	encWrapKey := encryptIVPrefixed(t, wrap1Key, wrap2Key)
	encFileKey := encryptIVPrefixed(t, fileKey, wrap1Key)

	var body bytes.Buffer
	for offset := 0; offset < len(payload); offset += 1008 {
		end := offset + 1008
		if end > len(payload) {
			end = len(payload)
		}
		body.Write(encryptIVPrefixed(t, payload[offset:end], fileKey))
	}

	keyMaterial := hex.EncodeToString(encFileKey) + ", " + hex.EncodeToString(encWrapKey)
	header := append(append(append(append([]byte{}, c1...), c2...), c3...), salt...)
	header = append(header, []byte(keyMaterial)...)

	out := make([]byte, 4, 4+len(header)+body.Len())
	binary.LittleEndian.PutUint32(out, uint32(len(header)))
	out = append(out, header...)
	out = append(out, body.Bytes()...)
	return out
}

func encryptIVPrefixed(t *testing.T, plain, key []byte) []byte {
	t.Helper()

	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}
	iv := randomBytes(t, block.BlockSize())
	padded := padPKCS7(plain, block.BlockSize())
	out := make([]byte, block.BlockSize()+len(padded))
	copy(out, iv)
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(out[block.BlockSize():], padded)
	return out
}

func padPKCS7(data []byte, size int) []byte {
	pad := size - len(data)%size
	out := make([]byte, len(data)+pad)
	copy(out, data)
	for i := len(data); i < len(out); i++ {
		out[i] = byte(pad)
	}
	return out
}

func randomBytes(t *testing.T, n int) []byte {
	t.Helper()
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return buf
}

func tarGz(t *testing.T, name string, content []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{
		Name:     name,
		Mode:     0o600,
		Size:     int64(len(content)),
		Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("write body: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
}

func TestDecryptXMLRoundTrip(t *testing.T) {
	payload := tarGz(t, "lastgood.xml", []byte(sampleXML))
	blob := buildContainer(t, payload)

	got, err := DecryptXML(blob)
	if err != nil {
		t.Fatalf("DecryptXML: %v", err)
	}
	if string(got) != sampleXML {
		t.Fatalf("round trip mismatch:\n got: %q\nwant: %q", got, sampleXML)
	}
}

// Payloads larger than one 1040-byte body chunk exercise the chunk loop.
func TestDecryptXMLHandlesMultipleChunks(t *testing.T) {
	big := strings.Repeat(sampleXML, 200)
	payload := tarGz(t, "lastgood.xml", []byte(big))
	blob := buildContainer(t, payload)

	got, err := DecryptXML(blob)
	if err != nil {
		t.Fatalf("DecryptXML: %v", err)
	}
	if string(got) != big {
		t.Fatalf("multi-chunk mismatch: got %d bytes, want %d", len(got), len(big))
	}
}

func TestParseRejectsMalformedInput(t *testing.T) {
	valid := buildContainer(t, tarGz(t, "lastgood.xml", []byte(sampleXML)))

	cases := []struct {
		name    string
		mutate  func([]byte) []byte
		wantErr string
	}{
		{
			name:    "too short",
			mutate:  func(b []byte) []byte { return b[:10] },
			wantErr: "too short",
		},
		{
			name: "header length beyond buffer",
			mutate: func(b []byte) []byte {
				out := append([]byte{}, b...)
				binary.LittleEndian.PutUint32(out, uint32(len(out)+100))
				return out
			},
			wantErr: "invalid header length",
		},
		{
			name: "body not block aligned",
			mutate: func(b []byte) []byte {
				return append(append([]byte{}, b...), 0x00)
			},
			wantErr: "invalid encrypted body length",
		},
		{
			name: "key material missing separator",
			mutate: func(b []byte) []byte {
				out := append([]byte{}, b...)
				headerLen := int(binary.LittleEndian.Uint32(out))
				for i := 4 + keyMaterialOff; i < 4+headerLen; i++ {
					if out[i] == ',' {
						out[i] = ';'
					}
				}
				return out
			},
			wantErr: "key material",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.mutate(append([]byte{}, valid...)))
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestExtractConfigXMLRejectsMultipleFiles(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, name := range []string{"a.xml", "b.xml"} {
		if err := tw.WriteHeader(&tar.Header{
			Name:     name,
			Mode:     0o600,
			Size:     2,
			Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatalf("write header: %v", err)
		}
		if _, err := tw.Write([]byte("x\n")); err != nil {
			t.Fatalf("write body: %v", err)
		}
	}
	tw.Close()
	gz.Close()

	_, err := ExtractConfigXML(buf.Bytes())
	if err == nil {
		t.Fatal("expected an error for a two-file archive")
	}
	if !strings.Contains(err.Error(), "found: a.xml, b.xml") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExtractConfigXMLRejectsGarbage(t *testing.T) {
	if _, err := ExtractConfigXML([]byte("not a gzip stream")); err == nil {
		t.Fatal("expected an error for non-archive input")
	}
}

func TestFindCredential(t *testing.T) {
	cred := FindCredential([]byte(sampleXML))
	if cred.User != "CMCCAdmin" || cred.Password != "aDm8H%MdA" {
		t.Fatalf("unexpected credential: %+v", cred)
	}
	if !cred.Complete() {
		t.Fatal("expected complete credential")
	}
}

func TestFindCredentialHandlesCDATAAndEntities(t *testing.T) {
	xml := "<root>\n" +
		"  <aucTeleAccountName><![CDATA[CMCCAdmin]]></aucTeleAccountName>\n" +
		"  <aucTeleAccountPassword>p&amp;ss&lt;word</aucTeleAccountPassword>\n" +
		"</root>"

	cred := FindCredential([]byte(xml))
	if cred.User != "CMCCAdmin" {
		t.Fatalf("CDATA user = %q", cred.User)
	}
	if cred.Password != "p&ss<word" {
		t.Fatalf("unescaped password = %q", cred.Password)
	}
}

func TestFindCredentialFromValueAttributes(t *testing.T) {
	// Shape used by the CMCC/Unionman firmware: one <Value> per setting, so the
	// account lives in attributes instead of its own element.
	xml := "<Config Name=\"ROOT\">\n" +
		" <Dir Name=\"TELECOM_ACCOUNT\">\n" +
		"  <Value Name=\"aucTeleAccountName\" Value=\"CMCCAdmin\"/>\n" +
		"  <Value Name=\"aucTeleAccountPassword\" Value=\"tc#TY1vB\"/>\n" +
		"  <Value Name=\"aucUserAccountName\" Value=\"user\"/>\n" +
		" </Dir>\n" +
		"</Config>"

	cred := FindCredential([]byte(xml))
	if cred.User != "CMCCAdmin" {
		t.Fatalf("user = %q", cred.User)
	}
	if cred.Password != "tc#TY1vB" {
		t.Fatalf("password = %q", cred.Password)
	}
}

func TestFindCredentialValueAttributesUnescape(t *testing.T) {
	xml := "<Config><Value Name='aucTeleAccountName' Value='CMCCAdmin'/>" +
		"<Value Value='p&amp;ss&lt;word' Name='aucTeleAccountPassword'/></Config>"

	cred := FindCredential([]byte(xml))
	if cred.User != "CMCCAdmin" || cred.Password != "p&ss<word" {
		t.Fatalf("unexpected credential: %+v", cred)
	}
}

func TestFindCredentialMissing(t *testing.T) {
	if FindCredential([]byte("<root/>")).Complete() {
		t.Fatal("expected an incomplete credential")
	}
}

// TestWriteFixture writes a synthetic encrypted container so the dlg binary can
// be exercised end to end without a real device:
//
//	DLG_FIXTURE_OUT=/tmp/lastgood.xml.enc go test ./internal/container -run TestWriteFixture
func TestWriteFixture(t *testing.T) {
	out := os.Getenv("DLG_FIXTURE_OUT")
	if out == "" {
		t.Skip("DLG_FIXTURE_OUT not set")
	}

	payload := tarGz(t, "lastgood.xml", []byte(sampleXML))
	if err := os.WriteFile(out, buildContainer(t, payload), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}
