package kdf

import (
	"encoding/hex"
	"testing"
)

// Test vectors from the widely published PBKDF2-HMAC-SHA256 corpus
// (password "password", salt "salt").
func TestSHA256Vectors(t *testing.T) {
	cases := []struct {
		name       string
		iterations int
		want       string
	}{
		{
			name:       "c=1",
			iterations: 1,
			want:       "120fb6cffcf8b32c43e7225256c4f837a86548c92ccc35480805987cb70be17b",
		},
		{
			name:       "c=2",
			iterations: 2,
			want:       "ae4d0c95af6b46d32d0adff928f06dd02a303f8ef3c251dfd6e2d85a95474c43",
		},
		{
			name:       "c=4096",
			iterations: 4096,
			want:       "c5e478d59288c841aa530db6845c4c8d962893a001ce4e11a4963873aa98134a",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := SHA256([]byte("password"), []byte("salt"), tc.iterations, 32)
			if err != nil {
				t.Fatalf("SHA256: %v", err)
			}
			if hex.EncodeToString(got) != tc.want {
				t.Fatalf("got %x, want %s", got, tc.want)
			}
		})
	}
}

// The device uses a 16-byte key, which must be the prefix of the 32-byte output
// because SHA-256 produces 32 bytes per block.
func TestSHA256ShortKeyIsPrefix(t *testing.T) {
	long, err := SHA256([]byte("secret"), []byte("salt"), 10000, 32)
	if err != nil {
		t.Fatalf("SHA256 long: %v", err)
	}
	short, err := SHA256([]byte("secret"), []byte("salt"), 10000, 16)
	if err != nil {
		t.Fatalf("SHA256 short: %v", err)
	}
	if hex.EncodeToString(short) != hex.EncodeToString(long[:16]) {
		t.Fatalf("16-byte key %x is not prefix of %x", short, long)
	}
}

func TestSHA256RejectsBadArguments(t *testing.T) {
	if _, err := SHA256([]byte("s"), []byte("s"), 0, 16); err == nil {
		t.Fatal("expected error for zero iterations")
	}
	if _, err := SHA256([]byte("s"), []byte("s"), 10, 0); err == nil {
		t.Fatal("expected error for zero key length")
	}
}
