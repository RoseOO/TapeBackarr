package auth

import (
	"path/filepath"
	"testing"

	"github.com/RoseOO/TapeBackarr/internal/database"
	"github.com/RoseOO/TapeBackarr/internal/models"
)

func setupTestDB(t *testing.T) *database.DB {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := database.New(dbPath)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}
	return db
}

func TestLogin(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	svc := NewService(db, "test-secret", 24)

	// Test login with default admin
	token, user, err := svc.Login("admin", "changeme")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}

	if token == "" {
		t.Error("expected non-empty token")
	}

	if user.Username != "admin" {
		t.Errorf("expected username 'admin', got '%s'", user.Username)
	}

	if user.Role != models.RoleAdmin {
		t.Errorf("expected role admin, got %s", user.Role)
	}
}

func TestLoginInvalidCredentials(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	svc := NewService(db, "test-secret", 24)

	// Test login with wrong password
	_, _, err := svc.Login("admin", "wrongpassword")
	if err != ErrInvalidCredentials {
		t.Errorf("expected ErrInvalidCredentials, got %v", err)
	}

	// Test login with non-existent user
	_, _, err = svc.Login("nonexistent", "password")
	if err != ErrInvalidCredentials {
		t.Errorf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestValidateToken(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	svc := NewService(db, "test-secret", 24)

	// Get token
	token, _, err := svc.Login("admin", "changeme")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}

	// Validate token
	claims, err := svc.ValidateToken(token)
	if err != nil {
		t.Fatalf("token validation failed: %v", err)
	}

	if claims.Username != "admin" {
		t.Errorf("expected username 'admin', got '%s'", claims.Username)
	}

	if claims.Role != models.RoleAdmin {
		t.Errorf("expected role admin, got %s", claims.Role)
	}
}

func TestValidateInvalidToken(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	svc := NewService(db, "test-secret", 24)

	// Test with garbage token
	_, err := svc.ValidateToken("invalid-token")
	if err != ErrInvalidToken {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}
}

func TestCreateUser(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	svc := NewService(db, "test-secret", 24)

	user, err := svc.CreateUser("testuser", "testpass", models.RoleOperator)
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	if user.Username != "testuser" {
		t.Errorf("expected username 'testuser', got '%s'", user.Username)
	}

	if user.Role != models.RoleOperator {
		t.Errorf("expected role operator, got %s", user.Role)
	}

	// Try to log in with new user
	_, _, err = svc.Login("testuser", "testpass")
	if err != nil {
		t.Fatalf("login with new user failed: %v", err)
	}
}

func TestCreateDuplicateUser(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	svc := NewService(db, "test-secret", 24)

	// Create first user
	_, err := svc.CreateUser("testuser", "pass1", models.RoleOperator)
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	// Try to create duplicate
	_, err = svc.CreateUser("testuser", "pass2", models.RoleReadOnly)
	if err != ErrUserExists {
		t.Errorf("expected ErrUserExists, got %v", err)
	}
}

func TestUpdatePassword(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	svc := NewService(db, "test-secret", 24)

	// Create user
	user, err := svc.CreateUser("testuser", "oldpass", models.RoleOperator)
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	// Update password
	err = svc.UpdatePassword(user.ID, "oldpass", "newpass")
	if err != nil {
		t.Fatalf("failed to update password: %v", err)
	}

	// Login with old password should fail
	_, _, err = svc.Login("testuser", "oldpass")
	if err != ErrInvalidCredentials {
		t.Errorf("expected ErrInvalidCredentials with old password, got %v", err)
	}

	// Login with new password should work
	_, _, err = svc.Login("testuser", "newpass")
	if err != nil {
		t.Fatalf("login with new password failed: %v", err)
	}
}

func TestDeleteUser(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	svc := NewService(db, "test-secret", 24)

	// Create user
	user, err := svc.CreateUser("testuser", "testpass", models.RoleOperator)
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	// Delete user
	err = svc.DeleteUser(user.ID)
	if err != nil {
		t.Fatalf("failed to delete user: %v", err)
	}

	// Try to login
	_, _, err = svc.Login("testuser", "testpass")
	if err != ErrInvalidCredentials {
		t.Errorf("expected ErrInvalidCredentials after deletion, got %v", err)
	}
}

func TestCheckPermission(t *testing.T) {
	tests := []struct {
		role       models.UserRole
		action     string
		shouldPass bool
	}{
		{models.RoleAdmin, "users.create", true},
		{models.RoleAdmin, "tapes.delete", true},
		{models.RoleAdmin, "restore.run", true},
		{models.RoleOperator, "users.create", false},
		{models.RoleOperator, "tapes.create", true},
		{models.RoleOperator, "restore.run", true},
		{models.RoleReadOnly, "tapes.read", true},
		{models.RoleReadOnly, "tapes.create", false},
		{models.RoleReadOnly, "restore.run", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.role)+"-"+tt.action, func(t *testing.T) {
			result := CheckPermission(tt.role, tt.action)
			if result != tt.shouldPass {
				t.Errorf("CheckPermission(%s, %s) = %v, want %v", tt.role, tt.action, result, tt.shouldPass)
			}
		})
	}
}
