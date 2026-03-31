package token

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTestKeyPair(t *testing.T) (privPath, pubPath string) {
	t.Helper()
	dir := t.TempDir()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	// Write private key
	privBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	privPath = filepath.Join(dir, "private.pem")
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privBytes})
	if err := os.WriteFile(privPath, privPEM, 0o600); err != nil {
		t.Fatalf("write private key: %v", err)
	}

	// Write public key
	pubBytes, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	pubPath = filepath.Join(dir, "public.pem")
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes})
	if err := os.WriteFile(pubPath, pubPEM, 0o644); err != nil {
		t.Fatalf("write public key: %v", err)
	}

	return privPath, pubPath
}

func TestGenerateES256(t *testing.T) {
	t.Parallel()

	privPath, _ := writeTestKeyPair(t)

	tests := []struct {
		name    string
		cfg     GenerateConfig
		wantSub string
	}{
		{
			name: "basic ES256 token",
			cfg: GenerateConfig{
				Subject:   "user123",
				TenantID:  "acme",
				Algorithm: "ES256",
				KeyFile:   privPath,
				TTL:       time.Hour,
			},
			wantSub: "user123",
		},
		{
			name: "with roles and groups",
			cfg: GenerateConfig{
				Subject:   "admin",
				TenantID:  "acme",
				Roles:     []string{"admin", "editor"},
				Groups:    []string{"team-a"},
				Algorithm: "ES256",
				KeyFile:   privPath,
			},
			wantSub: "admin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tokenStr, err := Generate(tt.cfg)
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}

			if tokenStr == "" {
				t.Fatal("empty token")
			}

			decoded, err := Decode(tokenStr)
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}

			if !decoded.Valid {
				t.Errorf("expected valid token, got error: %s", decoded.Error)
			}

			if sub, ok := decoded.Claims["sub"]; ok {
				if sub != tt.wantSub {
					t.Errorf("sub = %v, want %v", sub, tt.wantSub)
				}
			} else if tt.wantSub != "" {
				t.Error("expected sub claim")
			}
		})
	}
}

func TestValidateWithKeyFile(t *testing.T) {
	t.Parallel()

	privPath, pubPath := writeTestKeyPair(t)

	tokenStr, err := Generate(GenerateConfig{
		Subject:   "user",
		TenantID:  "acme",
		Algorithm: "ES256",
		KeyFile:   privPath,
		TTL:       time.Hour,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	result, err := ValidateWithKeyFile(tokenStr, pubPath)
	if err != nil {
		t.Fatalf("ValidateWithKeyFile: %v", err)
	}

	if !result.Valid {
		t.Errorf("expected valid, got error: %s", result.Error)
	}
}

func TestValidateWithKeyFile_Tampered(t *testing.T) {
	t.Parallel()

	privPath, pubPath := writeTestKeyPair(t)

	tokenStr, err := Generate(GenerateConfig{
		Subject:   "user",
		Algorithm: "ES256",
		KeyFile:   privPath,
		TTL:       time.Hour,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// Tamper with the token
	tampered := tokenStr[:len(tokenStr)-5] + "XXXXX"

	result, err := ValidateWithKeyFile(tampered, pubPath)
	if err != nil {
		t.Fatalf("ValidateWithKeyFile: %v", err)
	}

	if result.Valid {
		t.Error("expected invalid for tampered token")
	}
}

func TestDecodeExpiredToken(t *testing.T) {
	t.Parallel()

	privPath, _ := writeTestKeyPair(t)

	tokenStr, err := Generate(GenerateConfig{
		Subject:   "user",
		Algorithm: "ES256",
		KeyFile:   privPath,
		TTL:       -time.Hour, // already expired
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	decoded, err := Decode(tokenStr)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if decoded.Valid {
		t.Error("expected expired token to be invalid")
	}
	if decoded.Error != "token expired" {
		t.Errorf("error = %q, want 'token expired'", decoded.Error)
	}
}

func TestGenerateNoAlgorithm(t *testing.T) {
	t.Parallel()

	_, err := Generate(GenerateConfig{
		Subject: "user",
	})
	if err == nil {
		t.Error("expected error without algorithm")
	}
}

func TestGenerateNoKeyFile(t *testing.T) {
	t.Parallel()

	_, err := Generate(GenerateConfig{
		Subject:   "user",
		Algorithm: "ES256",
	})
	if err == nil {
		t.Error("expected error without key file")
	}
}

func TestGenerateUnsupportedAlgorithm(t *testing.T) {
	t.Parallel()

	privPath, _ := writeTestKeyPair(t)

	_, err := Generate(GenerateConfig{
		Subject:   "user",
		Algorithm: "HS256",
		KeyFile:   privPath,
	})
	if err == nil {
		t.Error("expected error for HS256 (removed)")
	}
}
