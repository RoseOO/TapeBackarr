package encryption

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/RoseOO/TapeBackarr/internal/database"
	"github.com/RoseOO/TapeBackarr/internal/logging"
)

// Algorithm represents the encryption algorithm
type Algorithm string

const (
	AlgorithmAES256GCM Algorithm = "aes-256-gcm"
)

// EncryptionKey represents an encryption key stored in the database
type EncryptionKey struct {
	ID             int64     `json:"id" db:"id"`
	Name           string    `json:"name" db:"name"`
	Algorithm      Algorithm `json:"algorithm" db:"algorithm"`
	KeyData        string    `json:"-" db:"key_data"`                      // Base64 encoded key (not exposed in JSON)
	KeyFingerprint string    `json:"key_fingerprint" db:"key_fingerprint"` // SHA256 fingerprint
	Description    string    `json:"description" db:"description"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
}

// KeySheet represents a printable key sheet for paper backup
type KeySheet struct {
	GeneratedAt  time.Time       `json:"generated_at"`
	Keys         []KeySheetEntry `json:"keys"`
	Instructions string          `json:"instructions"`
}

// KeySheetEntry represents a single key entry on the key sheet
type KeySheetEntry struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Fingerprint string `json:"fingerprint"`
	Algorithm   string `json:"algorithm"`
	KeyBase64   string `json:"key_base64"`
	CreatedAt   string `json:"created_at"`
}

// Service handles encryption operations
type Service struct {
	db     *database.DB
	logger *logging.Logger
}

// NewService creates a new encryption service
func NewService(db *database.DB, logger *logging.Logger) *Service {
	return &Service{
		db:     db,
		logger: logger,
	}
}

// GenerateKey generates a new AES-256 encryption key
func (s *Service) GenerateKey(ctx context.Context, name, description string) (*EncryptionKey, string, error) {
	// Generate 256-bit (32 bytes) random key
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, "", fmt.Errorf("failed to generate random key: %w", err)
	}

	keyBase64 := base64.StdEncoding.EncodeToString(key)
	fingerprint := s.calculateFingerprint(key)

	// Store the key
	result, err := s.db.Exec(`
		INSERT INTO encryption_keys (name, algorithm, key_data, key_fingerprint, description)
		VALUES (?, ?, ?, ?, ?)
	`, name, AlgorithmAES256GCM, keyBase64, fingerprint, description)
	if err != nil {
		return nil, "", fmt.Errorf("failed to store encryption key: %w", err)
	}

	keyID, _ := result.LastInsertId()

	s.logger.Info("Generated new encryption key", map[string]interface{}{
		"key_id":      keyID,
		"name":        name,
		"fingerprint": fingerprint,
	})

	encKey := &EncryptionKey{
		ID:             keyID,
		Name:           name,
		Algorithm:      AlgorithmAES256GCM,
		KeyData:        keyBase64,
		KeyFingerprint: fingerprint,
		Description:    description,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	// Return the key in base64 format for the user to save
	return encKey, keyBase64, nil
}

// GetKey retrieves an encryption key by ID
func (s *Service) GetKey(ctx context.Context, keyID int64) (*EncryptionKey, error) {
	var key EncryptionKey
	err := s.db.QueryRow(`
		SELECT id, name, algorithm, key_data, key_fingerprint, description, created_at, updated_at
		FROM encryption_keys
		WHERE id = ?
	`, keyID).Scan(&key.ID, &key.Name, &key.Algorithm, &key.KeyData, &key.KeyFingerprint, &key.Description, &key.CreatedAt, &key.UpdatedAt)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("encryption key not found: %d", keyID)
		}
		return nil, fmt.Errorf("failed to get encryption key: %w", err)
	}

	return &key, nil
}

// GetKeyByName retrieves an encryption key by name
func (s *Service) GetKeyByName(ctx context.Context, name string) (*EncryptionKey, error) {
	var key EncryptionKey
	err := s.db.QueryRow(`
		SELECT id, name, algorithm, key_data, key_fingerprint, description, created_at, updated_at
		FROM encryption_keys
		WHERE name = ?
	`, name).Scan(&key.ID, &key.Name, &key.Algorithm, &key.KeyData, &key.KeyFingerprint, &key.Description, &key.CreatedAt, &key.UpdatedAt)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("encryption key not found: %s", name)
		}
		return nil, fmt.Errorf("failed to get encryption key: %w", err)
	}

	return &key, nil
}

// ListKeys returns all encryption keys (without exposing the actual key data)
func (s *Service) ListKeys(ctx context.Context) ([]EncryptionKey, error) {
	rows, err := s.db.Query(`
		SELECT id, name, algorithm, key_fingerprint, description, created_at, updated_at
		FROM encryption_keys
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list encryption keys: %w", err)
	}
	defer rows.Close()

	var keys []EncryptionKey
	for rows.Next() {
		var key EncryptionKey
		if err := rows.Scan(&key.ID, &key.Name, &key.Algorithm, &key.KeyFingerprint, &key.Description, &key.CreatedAt, &key.UpdatedAt); err != nil {
			continue
		}
		keys = append(keys, key)
	}

	return keys, nil
}

// DeleteKey deletes an encryption key by ID
func (s *Service) DeleteKey(ctx context.Context, keyID int64) error {
	// Check if key is in use by any backup job
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM backup_jobs WHERE encryption_key_id = ?", keyID).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check key usage: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("encryption key is in use by %d backup job(s)", count)
	}

	// Check if key was used for any backup set
	err = s.db.QueryRow("SELECT COUNT(*) FROM backup_sets WHERE encryption_key_id = ?", keyID).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check key usage in backup sets: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("encryption key was used for %d backup set(s) - cannot delete", count)
	}

	_, err = s.db.Exec("DELETE FROM encryption_keys WHERE id = ?", keyID)
	if err != nil {
		return fmt.Errorf("failed to delete encryption key: %w", err)
	}

	s.logger.Info("Deleted encryption key", map[string]interface{}{
		"key_id": keyID,
	})

	return nil
}

// GenerateKeySheet creates a printable key sheet for paper backup
func (s *Service) GenerateKeySheet(ctx context.Context) (*KeySheet, error) {
	rows, err := s.db.Query(`
		SELECT id, name, algorithm, key_data, key_fingerprint, created_at
		FROM encryption_keys
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query encryption keys: %w", err)
	}
	defer rows.Close()

	sheet := &KeySheet{
		GeneratedAt: time.Now(),
		Keys:        []KeySheetEntry{},
		Instructions: `ENCRYPTION KEY BACKUP SHEET
==========================

IMPORTANT: Store this document in a secure location (safe, security deposit box).
This sheet contains encryption keys needed to restore encrypted backups.

To restore an encrypted backup without TapeBackarr:
1. Position tape to the encrypted backup set
2. Extract with: openssl enc -d -aes-256-gcm -pbkdf2 -pass pass:<key_base64> < /dev/nst0 | tar -xvf -
   OR use: gpg --decrypt --batch --passphrase <key_base64> < /dev/nst0 | tar -xvf -
3. See MANUAL_RECOVERY.md for detailed instructions

WARNING: Anyone with access to these keys can decrypt your backups.
`,
	}

	for rows.Next() {
		var entry KeySheetEntry
		var keyData string
		if err := rows.Scan(&entry.ID, &entry.Name, &entry.Algorithm, &keyData, &entry.Fingerprint, &entry.CreatedAt); err != nil {
			continue
		}
		entry.KeyBase64 = keyData
		sheet.Keys = append(sheet.Keys, entry)
	}

	return sheet, nil
}

// GenerateKeySheetText creates a plain text version of the key sheet for printing
func (s *Service) GenerateKeySheetText(ctx context.Context) (string, error) {
	sheet, err := s.GenerateKeySheet(ctx)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	buf.WriteString("=" + strings.Repeat("=", 78) + "\n")
	buf.WriteString("                    TAPEBACKARR ENCRYPTION KEY BACKUP\n")
	buf.WriteString("=" + strings.Repeat("=", 78) + "\n\n")
	buf.WriteString(fmt.Sprintf("Generated: %s\n\n", sheet.GeneratedAt.Format(time.RFC3339)))
	buf.WriteString(sheet.Instructions)
	buf.WriteString("\n\n")
	buf.WriteString("-" + strings.Repeat("-", 78) + "\n")
	buf.WriteString("                              KEY LISTING\n")
	buf.WriteString("-" + strings.Repeat("-", 78) + "\n\n")

	for i, key := range sheet.Keys {
		buf.WriteString(fmt.Sprintf("KEY #%d\n", i+1))
		buf.WriteString(fmt.Sprintf("  Name:        %s\n", key.Name))
		buf.WriteString(fmt.Sprintf("  ID:          %d\n", key.ID))
		buf.WriteString(fmt.Sprintf("  Algorithm:   %s\n", key.Algorithm))
		buf.WriteString(fmt.Sprintf("  Fingerprint: %s\n", key.Fingerprint))
		buf.WriteString(fmt.Sprintf("  Created:     %s\n", key.CreatedAt))
		buf.WriteString(fmt.Sprintf("  Key (Base64):\n"))
		// Split key into chunks for readability
		keyChunks := splitIntoChunks(key.KeyBase64, 44)
		for _, chunk := range keyChunks {
			buf.WriteString(fmt.Sprintf("    %s\n", chunk))
		}
		buf.WriteString("\n")
	}

	buf.WriteString("-" + strings.Repeat("-", 78) + "\n")
	buf.WriteString("                          END OF KEY LISTING\n")
	buf.WriteString("-" + strings.Repeat("-", 78) + "\n\n")
	buf.WriteString("Store this document securely. Destroy old copies when regenerating.\n")

	return buf.String(), nil
}

// Encrypt encrypts data using the specified key
func (s *Service) Encrypt(key *EncryptionKey, plaintext []byte) ([]byte, error) {
	keyBytes, err := base64.StdEncoding.DecodeString(key.KeyData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode key: %w", err)
	}

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Prepend nonce to ciphertext
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts data using the specified key
func (s *Service) Decrypt(key *EncryptionKey, ciphertext []byte) ([]byte, error) {
	keyBytes, err := base64.StdEncoding.DecodeString(key.KeyData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode key: %w", err)
	}

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plaintext, nil
}

// EncryptReader wraps a reader with encryption
func (s *Service) EncryptReader(key *EncryptionKey, r io.Reader) (io.Reader, error) {
	keyBytes, err := base64.StdEncoding.DecodeString(key.KeyData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode key: %w", err)
	}

	return NewEncryptingReader(r, keyBytes)
}

// DecryptReader wraps a reader with decryption
func (s *Service) DecryptReader(key *EncryptionKey, r io.Reader) (io.Reader, error) {
	keyBytes, err := base64.StdEncoding.DecodeString(key.KeyData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode key: %w", err)
	}

	return NewDecryptingReader(r, keyBytes)
}

// ImportKey imports an existing key from base64
func (s *Service) ImportKey(ctx context.Context, name, keyBase64, description string) (*EncryptionKey, error) {
	// Validate key
	keyBytes, err := base64.StdEncoding.DecodeString(keyBase64)
	if err != nil {
		return nil, fmt.Errorf("invalid key format: %w", err)
	}

	if len(keyBytes) != 32 {
		return nil, fmt.Errorf("key must be 32 bytes (256 bits), got %d bytes", len(keyBytes))
	}

	fingerprint := s.calculateFingerprint(keyBytes)

	// Store the key
	result, err := s.db.Exec(`
		INSERT INTO encryption_keys (name, algorithm, key_data, key_fingerprint, description)
		VALUES (?, ?, ?, ?, ?)
	`, name, AlgorithmAES256GCM, keyBase64, fingerprint, description)
	if err != nil {
		return nil, fmt.Errorf("failed to store encryption key: %w", err)
	}

	keyID, _ := result.LastInsertId()

	s.logger.Info("Imported encryption key", map[string]interface{}{
		"key_id":      keyID,
		"name":        name,
		"fingerprint": fingerprint,
	})

	return &EncryptionKey{
		ID:             keyID,
		Name:           name,
		Algorithm:      AlgorithmAES256GCM,
		KeyData:        keyBase64,
		KeyFingerprint: fingerprint,
		Description:    description,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}, nil
}

// calculateFingerprint calculates SHA256 fingerprint of the key
func (s *Service) calculateFingerprint(key []byte) string {
	hash := sha256.Sum256(key)
	return hex.EncodeToString(hash[:])
}

// GetKeyRawBytes retrieves the raw key bytes for a given key ID.
// This is used for hardware encryption where the raw AES-256 key must be
// sent to the tape drive firmware.
func (s *Service) GetKeyRawBytes(ctx context.Context, keyID int64) ([]byte, error) {
	key, err := s.GetKey(ctx, keyID)
	if err != nil {
		return nil, err
	}

	keyBytes, err := base64.StdEncoding.DecodeString(key.KeyData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode key data: %w", err)
	}

	return keyBytes, nil
}

// splitIntoChunks splits a string into chunks of specified size
func splitIntoChunks(s string, chunkSize int) []string {
	var chunks []string
	for len(s) > 0 {
		if len(s) < chunkSize {
			chunks = append(chunks, s)
			break
		}
		chunks = append(chunks, s[:chunkSize])
		s = s[chunkSize:]
	}
	return chunks
}
