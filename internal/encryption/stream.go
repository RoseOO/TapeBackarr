package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
)

const (
	// StreamChunkSize is the size of each encryption chunk (1MB to match the
	// default LTO block size and minimise per-chunk GCM overhead).
	StreamChunkSize = 1024 * 1024
	// NonceSize is the size of the GCM nonce (12 bytes)
	NonceSize = 12
	// TagSize is the size of the GCM authentication tag (16 bytes)
	TagSize = 16
	// ChunkOverhead is the nonce + tag size added to each encrypted chunk
	ChunkOverhead = NonceSize + TagSize
	// MagicHeader identifies encrypted streams
	MagicHeader = "TAPEBACKARR_ENC_V1"
)

// EncryptingReader wraps an io.Reader and encrypts data as it's read
type EncryptingReader struct {
	source     io.Reader
	gcm        cipher.AEAD
	buffer     []byte
	encrypted  []byte
	eof        bool
	headerSent bool
	headerBuf  []byte // Buffer for partial header sends
}

// NewEncryptingReader creates a new encrypting reader
func NewEncryptingReader(source io.Reader, key []byte) (*EncryptingReader, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	return &EncryptingReader{
		source:     source,
		gcm:        gcm,
		buffer:     make([]byte, StreamChunkSize),
		encrypted:  nil,
		eof:        false,
		headerSent: false,
		headerBuf:  []byte(MagicHeader),
	}, nil
}

// Read implements io.Reader
func (r *EncryptingReader) Read(p []byte) (int, error) {
	// First, send the magic header
	if len(r.headerBuf) > 0 {
		n := copy(p, r.headerBuf)
		r.headerBuf = r.headerBuf[n:]
		if len(r.headerBuf) == 0 {
			r.headerSent = true
		}
		return n, nil
	}

	// If we have encrypted data in buffer, return it
	if len(r.encrypted) > 0 {
		n := copy(p, r.encrypted)
		r.encrypted = r.encrypted[n:]
		return n, nil
	}

	if r.eof {
		return 0, io.EOF
	}

	// Read a chunk from source
	n, err := io.ReadFull(r.source, r.buffer)
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		r.eof = true
		if n == 0 {
			return 0, io.EOF
		}
	} else if err != nil {
		return 0, err
	}

	// Generate nonce
	nonce := make([]byte, r.gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return 0, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt the chunk
	ciphertext := r.gcm.Seal(nil, nonce, r.buffer[:n], nil)

	// Prepend chunk size (4 bytes big-endian) + nonce
	chunkSize := make([]byte, 4)
	size := uint32(len(ciphertext))
	chunkSize[0] = byte(size >> 24)
	chunkSize[1] = byte(size >> 16)
	chunkSize[2] = byte(size >> 8)
	chunkSize[3] = byte(size)

	r.encrypted = make([]byte, 0, 4+NonceSize+len(ciphertext))
	r.encrypted = append(r.encrypted, chunkSize...)
	r.encrypted = append(r.encrypted, nonce...)
	r.encrypted = append(r.encrypted, ciphertext...)

	// Return what fits in p
	copied := copy(p, r.encrypted)
	r.encrypted = r.encrypted[copied:]
	return copied, nil
}

// DecryptingReader wraps an io.Reader and decrypts data as it's read
type DecryptingReader struct {
	source     io.Reader
	gcm        cipher.AEAD
	decrypted  []byte
	eof        bool
	headerRead bool
}

// NewDecryptingReader creates a new decrypting reader
func NewDecryptingReader(source io.Reader, key []byte) (*DecryptingReader, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	return &DecryptingReader{
		source:     source,
		gcm:        gcm,
		decrypted:  nil,
		eof:        false,
		headerRead: false,
	}, nil
}

// Read implements io.Reader
func (r *DecryptingReader) Read(p []byte) (int, error) {
	// First, verify the magic header
	if !r.headerRead {
		header := make([]byte, len(MagicHeader))
		if _, err := io.ReadFull(r.source, header); err != nil {
			return 0, fmt.Errorf("failed to read encryption header: %w", err)
		}
		if string(header) != MagicHeader {
			return 0, fmt.Errorf("invalid encryption header: not an encrypted backup")
		}
		r.headerRead = true
	}

	// If we have decrypted data in buffer, return it
	if len(r.decrypted) > 0 {
		n := copy(p, r.decrypted)
		r.decrypted = r.decrypted[n:]
		return n, nil
	}

	if r.eof {
		return 0, io.EOF
	}

	// Read chunk size (4 bytes)
	sizeBuf := make([]byte, 4)
	_, err := io.ReadFull(r.source, sizeBuf)
	if err == io.EOF {
		r.eof = true
		return 0, io.EOF
	} else if err != nil {
		return 0, fmt.Errorf("failed to read chunk size: %w", err)
	}

	chunkSize := uint32(sizeBuf[0])<<24 | uint32(sizeBuf[1])<<16 | uint32(sizeBuf[2])<<8 | uint32(sizeBuf[3])

	// Validate chunk size
	if chunkSize > StreamChunkSize+ChunkOverhead+1024 {
		return 0, fmt.Errorf("chunk size too large: %d", chunkSize)
	}

	// Read nonce
	nonce := make([]byte, NonceSize)
	if _, err := io.ReadFull(r.source, nonce); err != nil {
		return 0, fmt.Errorf("failed to read nonce: %w", err)
	}

	// Read ciphertext
	ciphertext := make([]byte, chunkSize)
	if _, err := io.ReadFull(r.source, ciphertext); err != nil {
		return 0, fmt.Errorf("failed to read ciphertext: %w", err)
	}

	// Decrypt
	plaintext, err := r.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to decrypt chunk: %w", err)
	}

	r.decrypted = plaintext

	// Return what fits in p
	n := copy(p, r.decrypted)
	r.decrypted = r.decrypted[n:]
	return n, nil
}

// IsEncryptedStream checks if a reader starts with the encryption header
// Returns true if encrypted, false otherwise. The reader is NOT consumed.
func IsEncryptedStream(header []byte) bool {
	if len(header) < len(MagicHeader) {
		return false
	}
	return string(header[:len(MagicHeader)]) == MagicHeader
}
