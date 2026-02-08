package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/RoseOO/TapeBackarr/internal/database"
	"github.com/RoseOO/TapeBackarr/internal/models"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var (
	// ErrInvalidCredentials is returned when login fails
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrUserNotFound is returned when user doesn't exist
	ErrUserNotFound = errors.New("user not found")
	// ErrUserExists is returned when trying to create duplicate user
	ErrUserExists = errors.New("user already exists")
	// ErrInvalidToken is returned when token validation fails
	ErrInvalidToken = errors.New("invalid token")
	// ErrTokenExpired is returned when token has expired
	ErrTokenExpired = errors.New("token expired")
	// ErrInsufficientPermissions is returned when user lacks permission
	ErrInsufficientPermissions = errors.New("insufficient permissions")
)

// Claims represents JWT claims
type Claims struct {
	UserID   int64           `json:"user_id"`
	Username string          `json:"username"`
	Role     models.UserRole `json:"role"`
	jwt.RegisteredClaims
}

// Service handles authentication
type Service struct {
	db              *database.DB
	jwtSecret       []byte
	tokenExpiration time.Duration
}

// NewService creates a new auth service
func NewService(db *database.DB, jwtSecret string, tokenExpirationHours int) *Service {
	secret := []byte(jwtSecret)
	if len(secret) == 0 {
		// Generate random secret if not provided
		secret = make([]byte, 32)
		rand.Read(secret)
	}

	return &Service{
		db:              db,
		jwtSecret:       secret,
		tokenExpiration: time.Duration(tokenExpirationHours) * time.Hour,
	}
}

// Login authenticates a user and returns a JWT token
func (s *Service) Login(username, password string) (string, *models.User, error) {
	var user models.User
	err := s.db.QueryRow(`
		SELECT id, username, password_hash, role, created_at, updated_at
		FROM users WHERE username = ?
	`, username).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &user.CreatedAt, &user.UpdatedAt)

	if err != nil {
		return "", nil, ErrInvalidCredentials
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", nil, ErrInvalidCredentials
	}

	// Generate token
	token, err := s.generateToken(&user)
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate token: %w", err)
	}

	return token, &user, nil
}

// ValidateToken validates a JWT token and returns the claims
func (s *Service) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.jwtSecret, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

// generateToken generates a JWT token for a user
func (s *Service) generateToken(user *models.User) (string, error) {
	claims := &Claims{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.tokenExpiration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "tapebackarr",
			Subject:   fmt.Sprintf("%d", user.ID),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

// CreateUser creates a new user
func (s *Service) CreateUser(username, password string, role models.UserRole) (*models.User, error) {
	// Check if user exists
	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM users WHERE username = ?", username).Scan(&count)
	if count > 0 {
		return nil, ErrUserExists
	}

	// Hash password
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	result, err := s.db.Exec(`
		INSERT INTO users (username, password_hash, role)
		VALUES (?, ?, ?)
	`, username, string(hash), role)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	id, _ := result.LastInsertId()
	return &models.User{
		ID:       id,
		Username: username,
		Role:     role,
	}, nil
}

// UpdatePassword updates a user's password
func (s *Service) UpdatePassword(userID int64, oldPassword, newPassword string) error {
	var currentHash string
	err := s.db.QueryRow("SELECT password_hash FROM users WHERE id = ?", userID).Scan(&currentHash)
	if err != nil {
		return ErrUserNotFound
	}

	// Verify old password
	if err := bcrypt.CompareHashAndPassword([]byte(currentHash), []byte(oldPassword)); err != nil {
		return ErrInvalidCredentials
	}

	// Hash new password
	newHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	_, err = s.db.Exec(`
		UPDATE users SET password_hash = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, string(newHash), userID)

	return err
}

// GetUser returns a user by ID
func (s *Service) GetUser(userID int64) (*models.User, error) {
	var user models.User
	err := s.db.QueryRow(`
		SELECT id, username, role, created_at, updated_at
		FROM users WHERE id = ?
	`, userID).Scan(&user.ID, &user.Username, &user.Role, &user.CreatedAt, &user.UpdatedAt)

	if err != nil {
		return nil, ErrUserNotFound
	}

	return &user, nil
}

// ListUsers returns all users
func (s *Service) ListUsers() ([]models.User, error) {
	rows, err := s.db.Query(`
		SELECT id, username, role, created_at, updated_at
		FROM users ORDER BY username
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &u.CreatedAt, &u.UpdatedAt); err != nil {
			continue
		}
		users = append(users, u)
	}

	return users, nil
}

// ErrCannotDeleteAdmin is returned when trying to delete the default admin account
var ErrCannotDeleteAdmin = errors.New("cannot delete the default admin account")

// DeleteUser deletes a user
func (s *Service) DeleteUser(userID int64) error {
	// Prevent deleting the default admin account
	var username string
	err := s.db.QueryRow("SELECT username FROM users WHERE id = ?", userID).Scan(&username)
	if err != nil {
		return ErrUserNotFound
	}
	if username == "admin" {
		return ErrCannotDeleteAdmin
	}

	result, err := s.db.Exec("DELETE FROM users WHERE id = ?", userID)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrUserNotFound
	}

	return nil
}

// GenerateAPIKey creates a new API key and returns the raw key (only shown once)
func (s *Service) GenerateAPIKey(name string, role models.UserRole, expiresAt *time.Time) (string, *models.APIKey, error) {
	// Generate a random 32-byte key
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		return "", nil, fmt.Errorf("failed to generate key: %w", err)
	}
	rawKey := "tbk_" + hex.EncodeToString(keyBytes)
	keyPrefix := rawKey[:12]

	// Hash the key for storage
	hash, err := bcrypt.GenerateFromPassword([]byte(rawKey), bcrypt.DefaultCost)
	if err != nil {
		return "", nil, fmt.Errorf("failed to hash key: %w", err)
	}

	result, err := s.db.Exec(`
		INSERT INTO api_keys (name, key_hash, key_prefix, role, expires_at)
		VALUES (?, ?, ?, ?, ?)
	`, name, string(hash), keyPrefix, role, expiresAt)
	if err != nil {
		return "", nil, fmt.Errorf("failed to store API key: %w", err)
	}

	id, _ := result.LastInsertId()
	apiKey := &models.APIKey{
		ID:        id,
		Name:      name,
		KeyPrefix: keyPrefix,
		Role:      role,
		ExpiresAt: expiresAt,
	}

	return rawKey, apiKey, nil
}

// ValidateAPIKey validates an API key and returns claims if valid
func (s *Service) ValidateAPIKey(rawKey string) (*Claims, error) {
	if len(rawKey) < 12 {
		return nil, ErrInvalidToken
	}
	prefix := rawKey[:12]

	var apiKey models.APIKey
	err := s.db.QueryRow(`
		SELECT id, name, key_hash, key_prefix, role, expires_at
		FROM api_keys WHERE key_prefix = ?
	`, prefix).Scan(&apiKey.ID, &apiKey.Name, &apiKey.KeyHash, &apiKey.KeyPrefix, &apiKey.Role, &apiKey.ExpiresAt)
	if err != nil {
		return nil, ErrInvalidToken
	}

	// Check expiration
	if apiKey.ExpiresAt != nil && apiKey.ExpiresAt.Before(time.Now()) {
		return nil, ErrTokenExpired
	}

	// Verify key hash
	if err := bcrypt.CompareHashAndPassword([]byte(apiKey.KeyHash), []byte(rawKey)); err != nil {
		return nil, ErrInvalidCredentials
	}

	// Update last used timestamp
	s.db.Exec("UPDATE api_keys SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?", apiKey.ID)

	return &Claims{
		UserID:   -apiKey.ID, // Negative to distinguish from user IDs
		Username: "api:" + apiKey.Name,
		Role:     apiKey.Role,
	}, nil
}

// ListAPIKeys returns all API keys (without hashes)
func (s *Service) ListAPIKeys() ([]models.APIKey, error) {
	rows, err := s.db.Query(`
		SELECT id, name, key_prefix, role, last_used_at, expires_at, created_at
		FROM api_keys ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []models.APIKey
	for rows.Next() {
		var k models.APIKey
		if err := rows.Scan(&k.ID, &k.Name, &k.KeyPrefix, &k.Role, &k.LastUsedAt, &k.ExpiresAt, &k.CreatedAt); err != nil {
			continue
		}
		keys = append(keys, k)
	}
	return keys, nil
}

// DeleteAPIKey deletes an API key
func (s *Service) DeleteAPIKey(id int64) error {
	_, err := s.db.Exec("DELETE FROM api_keys WHERE id = ?", id)
	return err
}

// CheckPermission checks if a role has permission for an action
func CheckPermission(role models.UserRole, action string) bool {
	permissions := map[models.UserRole][]string{
		models.RoleAdmin: {
			"users.create", "users.delete", "users.update",
			"tapes.create", "tapes.delete", "tapes.update", "tapes.read",
			"jobs.create", "jobs.delete", "jobs.update", "jobs.run", "jobs.read",
			"sources.create", "sources.delete", "sources.update", "sources.read",
			"restore.run", "restore.read",
			"logs.read", "logs.export",
			"settings.update", "settings.read",
		},
		models.RoleOperator: {
			"tapes.create", "tapes.update", "tapes.read",
			"jobs.create", "jobs.update", "jobs.run", "jobs.read",
			"sources.create", "sources.update", "sources.read",
			"restore.run", "restore.read",
			"logs.read",
			"settings.read",
		},
		models.RoleReadOnly: {
			"tapes.read",
			"jobs.read",
			"sources.read",
			"restore.read",
			"logs.read",
			"settings.read",
		},
	}

	allowed, ok := permissions[role]
	if !ok {
		return false
	}

	for _, perm := range allowed {
		if perm == action {
			return true
		}
	}

	return false
}

// GenerateAPIKey generates a random API key
func GenerateAPIKey() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
