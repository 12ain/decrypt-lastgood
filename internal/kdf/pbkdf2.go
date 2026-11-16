// Package kdf implements PBKDF2-HMAC-SHA256 using only the standard library,
// so the whole tool can be built without third-party modules.
package kdf

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"errors"
)

// SHA256 derives keyLen bytes from secret and salt using PBKDF2-HMAC-SHA256.
func SHA256(secret, salt []byte, iterations, keyLen int) ([]byte, error) {
	if iterations < 1 {
		return nil, errors.New("pbkdf2: iterations must be at least 1")
	}
	if keyLen < 1 {
		return nil, errors.New("pbkdf2: key length must be at least 1")
	}

	prf := hmac.New(sha256.New, secret)
	hashLen := prf.Size()
	blocks := (keyLen + hashLen - 1) / hashLen

	derived := make([]byte, 0, blocks*hashLen)
	var index [4]byte

	for block := 1; block <= blocks; block++ {
		prf.Reset()
		prf.Write(salt)
		binary.BigEndian.PutUint32(index[:], uint32(block))
		prf.Write(index[:])

		u := prf.Sum(nil)
		t := make([]byte, len(u))
		copy(t, u)

		for n := 1; n < iterations; n++ {
			prf.Reset()
			prf.Write(u)
			u = prf.Sum(u[:0])
			for i := range t {
				t[i] ^= u[i]
			}
		}
		derived = append(derived, t...)
	}

	return derived[:keyLen], nil
}
