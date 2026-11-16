// Package container parses and decrypts the Unionman/CMCC lastgood.xml backup
// container produced by the UNG953H1-S optical modem.
//
// Wire format:
//
//	offset 0   4 bytes   header length, little endian uint32
//	offset 4   128 bytes component 1
//	offset 132 128 bytes component 2
//	offset 260 128 bytes component 3
//	offset 388 128 bytes salt
//	offset 516 ...       ASCII "<file key hex>, <wrap key hex>"
//	offset 4+headerLen   AES-CBC encrypted body, length must be a multiple of 16
//
// The three 128-byte components are XORed together to form the PBKDF2 secret.
// The derived key unwraps the wrap key, which in turn unwraps the file key.
// The body is then decrypted in 1040-byte chunks, each chunk being a 16-byte IV
// followed by at most 1024 bytes of ciphertext.
package container

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/12ain/decrypt-lastgood/internal/kdf"
)

const (
	componentSize  = 128
	componentCount = 3
	saltOffset     = componentSize * componentCount
	keyMaterialOff = saltOffset + componentSize
	minHeaderSize  = keyMaterialOff + 2
	bodyChunkSize  = 16 + 1024
	pbkdf2Rounds   = 10000
	wrapKeyLen     = 16
	fileKeyLen     = 16
)

var errPaddding = errors.New("AES padding check failed")

// Container is the parsed, still-encrypted backup file.
type Container struct {
	HeaderLen  int
	Component1 []byte
	Component2 []byte
	Component3 []byte
	Salt       []byte
	EncFileKey []byte
	EncWrapKey []byte
	Body       []byte
}

// Parse validates and splits the container without decrypting it.
func Parse(data []byte) (*Container, error) {
	if len(data) < 4+minHeaderSize {
		return nil, fmt.Errorf("input is too short to be a lastgood container (%d bytes)", len(data))
	}

	headerLen := int(binary.LittleEndian.Uint32(data[:4]))
	if headerLen < minHeaderSize || 4+headerLen > len(data) {
		return nil, fmt.Errorf("invalid header length: %d", headerLen)
	}

	header := data[4 : 4+headerLen]
	body := data[4+headerLen:]
	if len(body) == 0 || len(body)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("invalid encrypted body length: %d", len(body))
	}

	fileKey, wrapKey, err := parseKeyMaterial(header[keyMaterialOff:])
	if err != nil {
		return nil, err
	}

	return &Container{
		HeaderLen:  headerLen,
		Component1: header[0:componentSize],
		Component2: header[componentSize : 2*componentSize],
		Component3: header[2*componentSize : saltOffset],
		Salt:       header[saltOffset:keyMaterialOff],
		EncFileKey: fileKey,
		EncWrapKey: wrapKey,
		Body:       body,
	}, nil
}

func parseKeyMaterial(raw []byte) (fileKey, wrapKey []byte, err error) {
	malformed := errors.New("header key material is not in the expected format")

	parts := strings.SplitN(string(raw), ", ", 2)
	if len(parts) != 2 {
		return nil, nil, malformed
	}

	fileKey, err = hex.DecodeString(strings.TrimSpace(parts[0]))
	if err != nil {
		return nil, nil, malformed
	}
	wrapKey, err = hex.DecodeString(strings.TrimSpace(parts[1]))
	if err != nil {
		return nil, nil, malformed
	}
	return fileKey, wrapKey, nil
}

// DecryptContainer returns the decrypted tar.gz payload.
func DecryptContainer(data []byte) ([]byte, error) {
	c, err := Parse(data)
	if err != nil {
		return nil, err
	}

	secret := make([]byte, componentSize)
	for i := 0; i < componentSize; i++ {
		secret[i] = c.Component1[i] ^ c.Component2[i] ^ c.Component3[i]
	}

	wrap2Key, err := kdf.SHA256(secret, c.Salt, pbkdf2Rounds, wrapKeyLen)
	if err != nil {
		return nil, fmt.Errorf("derive key: %w", err)
	}

	wrap1Key, err := decryptIVPrefixed(c.EncWrapKey, wrap2Key)
	if err != nil {
		return nil, fmt.Errorf("unwrap wrap key: %w", err)
	}

	fileKey, err := decryptIVPrefixed(c.EncFileKey, wrap1Key)
	if err != nil {
		return nil, fmt.Errorf("unwrap file key: %w", err)
	}
	if len(fileKey) != fileKeyLen {
		return nil, fmt.Errorf("unexpected file key length: %d", len(fileKey))
	}

	var plain bytes.Buffer
	for offset := 0; offset < len(c.Body); offset += bodyChunkSize {
		end := offset + bodyChunkSize
		if end > len(c.Body) {
			end = len(c.Body)
		}
		chunk, err := decryptIVPrefixed(c.Body[offset:end], fileKey)
		if err != nil {
			return nil, fmt.Errorf("decrypt body chunk at offset %d: %w", offset, err)
		}
		plain.Write(chunk)
	}

	return plain.Bytes(), nil
}

// ExtractConfigXML pulls the single regular file out of the decrypted tar.gz.
func ExtractConfigXML(tarGz []byte) ([]byte, error) {
	notAnArchive := errors.New("decrypted data is not the expected tar.gz archive")

	gz, err := gzip.NewReader(bytes.NewReader(tarGz))
	if err != nil {
		return nil, notAnArchive
	}
	defer gz.Close()

	reader := tar.NewReader(gz)
	var names []string
	var content []byte

	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, notAnArchive
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}

		names = append(names, header.Name)
		if len(names) > 1 {
			continue
		}
		content, err = io.ReadAll(reader)
		if err != nil {
			return nil, fmt.Errorf("read archive member %q: %w", header.Name, err)
		}
	}

	switch len(names) {
	case 0:
		return nil, notAnArchive
	case 1:
		return content, nil
	default:
		return nil, fmt.Errorf("expected one file in archive, found: %s", strings.Join(names, ", "))
	}
}

// DecryptXML is the full pipeline: container -> tar.gz -> config XML.
func DecryptXML(data []byte) ([]byte, error) {
	packed, err := DecryptContainer(data)
	if err != nil {
		return nil, err
	}
	return ExtractConfigXML(packed)
}

// decryptIVPrefixed decrypts a blob whose first block is the IV and strips
// PKCS#7 padding afterwards.
func decryptIVPrefixed(data, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("invalid AES key: %w", err)
	}

	size := block.BlockSize()
	if len(data) < 2*size || len(data)%size != 0 {
		return nil, fmt.Errorf("invalid AES blob length: %d", len(data))
	}

	plain := make([]byte, len(data)-size)
	cipher.NewCBCDecrypter(block, data[:size]).CryptBlocks(plain, data[size:])

	return unpadPKCS7(plain, size)
}

func unpadPKCS7(data []byte, size int) ([]byte, error) {
	if len(data) == 0 || len(data)%size != 0 {
		return nil, errPaddding
	}

	pad := int(data[len(data)-1])
	if pad == 0 || pad > size || pad > len(data) {
		return nil, errPaddding
	}
	for _, b := range data[len(data)-pad:] {
		if int(b) != pad {
			return nil, errPaddding
		}
	}

	return data[:len(data)-pad], nil
}
