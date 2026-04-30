package trace

import (
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const encryptionMagic = "APERIO-ENC-V1\n"

// WriteTrace writes a trace to the given path atomically.
// If the path ends in .gz, the trace is gzip-compressed.
func WriteTrace(path string, t *Trace) error {
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal trace: %w", err)
	}

	// Compress if .gz extension
	if strings.HasSuffix(path, ".gz") {
		return writeCompressed(path, data)
	}

	return writeAtomic(path, data)
}

func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "aperio-trace-*.json")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}

func writeCompressed(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "aperio-trace-*.json.gz")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	gz := gzip.NewWriter(tmp)
	if _, err := gz.Write(data); err != nil {
		gz.Close()
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write compressed: %w", err)
	}
	if err := gz.Close(); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("close gzip: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}

// ReadTrace reads a trace from the given path.
// It auto-detects gzip-compressed files (.gz) and encrypted files (magic header).
func ReadTrace(path string) (*Trace, error) {
	if strings.HasSuffix(path, ".gz") {
		return readCompressed(path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read trace file: %w", err)
	}

	if len(data) > len(encryptionMagic) && string(data[:len(encryptionMagic)]) == encryptionMagic {
		return nil, fmt.Errorf("trace file is encrypted; use ReadTraceEncrypted with the decryption key")
	}

	var t Trace
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("unmarshal trace: %w", err)
	}

	return &t, nil
}

func readCompressed(path string) (*Trace, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open compressed trace: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("create gzip reader: %w", err)
	}
	defer gz.Close()

	data, err := io.ReadAll(gz)
	if err != nil {
		return nil, fmt.Errorf("read compressed trace: %w", err)
	}

	var t Trace
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("unmarshal trace: %w", err)
	}

	return &t, nil
}

// WriteTraceEncrypted writes a trace encrypted with AES-256-GCM.
// The key must be exactly 32 bytes.
func WriteTraceEncrypted(path string, t *Trace, key []byte) error {
	if len(key) != 32 {
		return fmt.Errorf("encryption key must be 32 bytes, got %d", len(key))
	}

	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal trace: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, data, nil)

	// Write with magic header
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "aperio-trace-enc-*.json")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write([]byte(encryptionMagic)); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write magic header: %w", err)
	}
	if _, err := tmp.Write(ciphertext); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write ciphertext: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}

// ReadTraceEncrypted reads an AES-256-GCM encrypted trace file.
// The key must be exactly 32 bytes.
func ReadTraceEncrypted(path string, key []byte) (*Trace, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("encryption key must be 32 bytes, got %d", len(key))
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read trace file: %w", err)
	}

	magicLen := len(encryptionMagic)
	if len(data) < magicLen || string(data[:magicLen]) != encryptionMagic {
		return nil, fmt.Errorf("file does not have encryption magic header")
	}

	ciphertext := data[magicLen:]

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt trace: %w (wrong key?)", err)
	}

	var t Trace
	if err := json.Unmarshal(plaintext, &t); err != nil {
		return nil, fmt.Errorf("unmarshal decrypted trace: %w", err)
	}

	return &t, nil
}
