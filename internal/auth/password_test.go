package auth

import (
	"testing"
)

func TestHashPassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{
			name:     "Valid password",
			password: "mySecurePassword123",
			wantErr:  false,
		},
		{
			name:     "Empty password",
			password: "",
			wantErr:  true, // Minimum 8 characters enforced
		},
		{
			name:     "Long password",
			password: "this-is-a-very-long-password-that-should-still-work-fine-1234567890",
			wantErr:  false,
		},
		{
			name:     "Special characters",
			password: "p@ssw0rd!#$%^&*()",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := HashPassword(tt.password)
			if (err != nil) != tt.wantErr {
				t.Errorf("HashPassword() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && hash == "" {
				t.Error("HashPassword() returned empty hash")
			}
			// Verify hash starts with BCrypt identifier
			if !tt.wantErr && len(hash) > 0 && hash[:4] != "$2a$" {
				t.Errorf("HashPassword() hash doesn't start with BCrypt identifier, got %s", hash[:4])
			}
		})
	}
}

func TestVerifyPassword(t *testing.T) {
	// Generate a known hash for testing
	testPassword := "testPassword123"
	hash, err := HashPassword(testPassword)
	if err != nil {
		t.Fatalf("Failed to hash password for test: %v", err)
	}

	tests := []struct {
		name     string
		hash     string
		password string
		wantErr  bool
	}{
		{
			name:     "Correct password",
			hash:     hash,
			password: testPassword,
			wantErr:  false,
		},
		{
			name:     "Incorrect password",
			hash:     hash,
			password: "wrongPassword",
			wantErr:  true,
		},
		{
			name:     "Empty password",
			hash:     hash,
			password: "",
			wantErr:  true,
		},
		{
			name:     "Case sensitive",
			hash:     hash,
			password: "TestPassword123", // Different case
			wantErr:  true,
		},
		{
			name:     "Invalid hash format",
			hash:     "invalid-hash",
			password: testPassword,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifyPassword(tt.hash, tt.password)
			if (err != nil) != tt.wantErr {
				t.Errorf("VerifyPassword() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestHashPasswordDeterminism(t *testing.T) {
	// Hashing the same password should produce different hashes (salt)
	password := "testPassword"
	hash1, err1 := HashPassword(password)
	hash2, err2 := HashPassword(password)

	if err1 != nil || err2 != nil {
		t.Fatalf("HashPassword() failed: %v, %v", err1, err2)
	}

	if hash1 == hash2 {
		t.Error("HashPassword() produced identical hashes for same password (salt not working)")
	}

	// But both should verify correctly
	if err := VerifyPassword(hash1, password); err != nil {
		t.Error("First hash failed to verify")
	}
	if err := VerifyPassword(hash2, password); err != nil {
		t.Error("Second hash failed to verify")
	}
}

func TestValidatePasswordStrength(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{
			name:     "Valid 8 character password",
			password: "password",
			wantErr:  false,
		},
		{
			name:     "Valid long password",
			password: "this-is-a-very-long-password",
			wantErr:  false,
		},
		{
			name:     "Too short - 7 characters",
			password: "short12",
			wantErr:  true,
		},
		{
			name:     "Too short - empty",
			password: "",
			wantErr:  true,
		},
		{
			name:     "Exactly minimum length",
			password: "12345678",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePasswordStrength(tt.password)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePasswordStrength() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestHashPasswordWithCost(t *testing.T) {
	tests := []struct {
		name     string
		password string
		cost     int
		wantErr  bool
	}{
		{
			name:     "Valid password with default cost",
			password: "myPassword123",
			cost:     10,
			wantErr:  false,
		},
		{
			name:     "Valid password with low cost",
			password: "myPassword123",
			cost:     4,
			wantErr:  false,
		},
		{
			name:     "Too short password",
			password: "short",
			cost:     10,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := HashPasswordWithCost(tt.password, tt.cost)
			if (err != nil) != tt.wantErr {
				t.Errorf("HashPasswordWithCost() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && hash == "" {
				t.Error("HashPasswordWithCost() returned empty hash")
			}
		})
	}
}
