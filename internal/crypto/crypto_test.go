package crypto

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestEncryptDecryptRoundtrip(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}

	plaintext := "hello, world! this is a secret"
	ciphertext, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	if ciphertext == plaintext {
		t.Fatal("Encrypt returned plaintext unchanged")
	}

	decrypted, err := Decrypt(key, ciphertext)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if decrypted != plaintext {
		t.Fatalf("Roundtrip mismatch: got %q, want %q", decrypted, plaintext)
	}
}

func TestDecryptToleratesPlaintext(t *testing.T) {
	key := make([]byte, 32)

	_, err := Decrypt(key, "this is not base64 encoded ciphertext!!!")
	if err == nil {
		t.Fatal("expected error when decrypting plaintext, got nil")
	}
}

func TestLoadOrCreateKey(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "test.key")

	// First call: file doesn't exist yet — should create it
	key1, err := LoadOrCreateKey(keyPath)
	if err != nil {
		t.Fatalf("first LoadOrCreateKey failed: %v", err)
	}
	if len(key1) != 32 {
		t.Fatalf("expected 32-byte key, got %d bytes", len(key1))
	}

	// Second call: file exists — should read same key
	key2, err := LoadOrCreateKey(keyPath)
	if err != nil {
		t.Fatalf("second LoadOrCreateKey failed: %v", err)
	}
	if len(key2) != 32 {
		t.Fatalf("expected 32-byte key on second load, got %d bytes", len(key2))
	}

	if !bytes.Equal(key1, key2) {
		t.Fatal("keys from first and second LoadOrCreateKey calls do not match")
	}
}
