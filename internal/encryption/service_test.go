package encryption

import (
	"bytes"
	"io"
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	// Generate a test key (32 bytes for AES-256)
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	plaintext := []byte("Hello, this is a test message for encryption!")

	// Test basic encrypt/decrypt
	t.Run("BasicEncryptDecrypt", func(t *testing.T) {
		// Create an encrypting reader
		encReader, err := NewEncryptingReader(bytes.NewReader(plaintext), key)
		if err != nil {
			t.Fatalf("Failed to create encrypting reader: %v", err)
		}

		// Read all encrypted data
		encrypted, err := io.ReadAll(encReader)
		if err != nil {
			t.Fatalf("Failed to read encrypted data: %v", err)
		}

		// Verify magic header is present
		if len(encrypted) < len(MagicHeader) {
			t.Fatalf("Encrypted data too short")
		}
		if string(encrypted[:len(MagicHeader)]) != MagicHeader {
			t.Errorf("Magic header not found")
		}

		// Create a decrypting reader
		decReader, err := NewDecryptingReader(bytes.NewReader(encrypted), key)
		if err != nil {
			t.Fatalf("Failed to create decrypting reader: %v", err)
		}

		// Read all decrypted data
		decrypted, err := io.ReadAll(decReader)
		if err != nil {
			t.Fatalf("Failed to read decrypted data: %v", err)
		}

		// Compare
		if !bytes.Equal(plaintext, decrypted) {
			t.Errorf("Decrypted data does not match original.\nExpected: %s\nGot: %s", plaintext, decrypted)
		}
	})

	// Test with larger data
	t.Run("LargerData", func(t *testing.T) {
		// Generate larger test data (128KB - larger than chunk size)
		largeData := make([]byte, 128*1024)
		for i := range largeData {
			largeData[i] = byte(i % 256)
		}

		// Encrypt
		encReader, err := NewEncryptingReader(bytes.NewReader(largeData), key)
		if err != nil {
			t.Fatalf("Failed to create encrypting reader: %v", err)
		}

		encrypted, err := io.ReadAll(encReader)
		if err != nil {
			t.Fatalf("Failed to read encrypted data: %v", err)
		}

		// Decrypt
		decReader, err := NewDecryptingReader(bytes.NewReader(encrypted), key)
		if err != nil {
			t.Fatalf("Failed to create decrypting reader: %v", err)
		}

		decrypted, err := io.ReadAll(decReader)
		if err != nil {
			t.Fatalf("Failed to read decrypted data: %v", err)
		}

		if !bytes.Equal(largeData, decrypted) {
			t.Errorf("Large data decryption failed. Expected %d bytes, got %d bytes", len(largeData), len(decrypted))
		}
	})

	// Test empty data
	t.Run("EmptyData", func(t *testing.T) {
		emptyData := []byte{}

		encReader, err := NewEncryptingReader(bytes.NewReader(emptyData), key)
		if err != nil {
			t.Fatalf("Failed to create encrypting reader: %v", err)
		}

		encrypted, err := io.ReadAll(encReader)
		if err != nil {
			t.Fatalf("Failed to read encrypted data: %v", err)
		}

		// Should at least have the magic header
		if len(encrypted) < len(MagicHeader) {
			t.Errorf("Encrypted empty data should have at least magic header")
		}

		decReader, err := NewDecryptingReader(bytes.NewReader(encrypted), key)
		if err != nil {
			t.Fatalf("Failed to create decrypting reader: %v", err)
		}

		decrypted, err := io.ReadAll(decReader)
		if err != nil {
			t.Fatalf("Failed to read decrypted data: %v", err)
		}

		if len(decrypted) != 0 {
			t.Errorf("Expected empty decrypted data, got %d bytes", len(decrypted))
		}
	})
}

func TestIsEncryptedStream(t *testing.T) {
	tests := []struct {
		name     string
		header   []byte
		expected bool
	}{
		{
			name:     "ValidHeader",
			header:   []byte(MagicHeader + "extra data"),
			expected: true,
		},
		{
			name:     "InvalidHeader",
			header:   []byte("not encrypted data"),
			expected: false,
		},
		{
			name:     "TooShort",
			header:   []byte("TAPE"),
			expected: false,
		},
		{
			name:     "Empty",
			header:   []byte{},
			expected: false,
		},
		{
			name:     "TarHeader",
			header:   []byte("./path/to/file\x00\x00\x00"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsEncryptedStream(tt.header)
			if result != tt.expected {
				t.Errorf("IsEncryptedStream(%q) = %v, want %v", tt.header, result, tt.expected)
			}
		})
	}
}

func TestChunkedEncryption(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i * 2)
	}

	// Test that reading in different chunk sizes still works
	originalData := []byte("This is test data that will be encrypted and read in different chunk sizes.")

	encReader, err := NewEncryptingReader(bytes.NewReader(originalData), key)
	if err != nil {
		t.Fatalf("Failed to create encrypting reader: %v", err)
	}

	// Read encrypted data in small chunks
	var encrypted []byte
	buf := make([]byte, 7) // Odd size to test edge cases
	for {
		n, err := encReader.Read(buf)
		if n > 0 {
			encrypted = append(encrypted, buf[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Error reading encrypted data: %v", err)
		}
	}

	// Decrypt in small chunks too
	decReader, err := NewDecryptingReader(bytes.NewReader(encrypted), key)
	if err != nil {
		t.Fatalf("Failed to create decrypting reader: %v", err)
	}

	var decrypted []byte
	smallBuf := make([]byte, 5)
	for {
		n, err := decReader.Read(smallBuf)
		if n > 0 {
			decrypted = append(decrypted, smallBuf[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Error reading decrypted data: %v", err)
		}
	}

	if !bytes.Equal(originalData, decrypted) {
		t.Errorf("Chunked decryption failed.\nExpected: %s\nGot: %s", originalData, decrypted)
	}
}

func TestWrongKeyDecryption(t *testing.T) {
	correctKey := make([]byte, 32)
	wrongKey := make([]byte, 32)
	for i := range correctKey {
		correctKey[i] = byte(i)
		wrongKey[i] = byte(i + 1) // Different key
	}

	plaintext := []byte("Secret message")

	// Encrypt with correct key
	encReader, _ := NewEncryptingReader(bytes.NewReader(plaintext), correctKey)
	encrypted, _ := io.ReadAll(encReader)

	// Try to decrypt with wrong key
	decReader, _ := NewDecryptingReader(bytes.NewReader(encrypted), wrongKey)
	_, err := io.ReadAll(decReader)

	// Should fail with decryption error
	if err == nil {
		t.Error("Expected decryption to fail with wrong key, but it succeeded")
	}
}
