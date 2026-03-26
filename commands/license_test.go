package commands

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestDecodeLicenseClaims_Valid(t *testing.T) {
	t.Parallel()

	claims := licenseClaims{
		Edition: "pro",
		Org:     "Acme Corp",
		Exp:     time.Date(2027, 3, 25, 0, 0, 0, 0, time.UTC).Unix(),
	}
	payload, _ := json.Marshal(claims)
	key := base64.RawURLEncoding.EncodeToString(payload) + "." +
		base64.RawURLEncoding.EncodeToString([]byte("fake-signature"))

	got, err := decodeLicenseClaims(key)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Edition != "pro" {
		t.Errorf("edition = %q, want pro", got.Edition)
	}
	if got.Org != "Acme Corp" {
		t.Errorf("org = %q, want Acme Corp", got.Org)
	}
	if got.Exp != claims.Exp {
		t.Errorf("exp = %d, want %d", got.Exp, claims.Exp)
	}
}

func TestDecodeLicenseClaims_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		key  string
	}{
		{"no dot separator", "nodot"},
		{"bad base64 payload", "!!!.dGVzdA"},
		{"bad base64 signature", "dGVzdA.!!!"},
		{"invalid JSON payload", base64.RawURLEncoding.EncodeToString([]byte("not-json")) + ".dGVzdA"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := decodeLicenseClaims(tt.key)
			if err == nil {
				t.Error("expected error but got nil")
			}
		})
	}
}

func TestMaskKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"short", "***"},
		{"exactly12ch", "***"},
		{"abcdefghijklmnop.signature", "abcdefgh...ture"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			if got := maskKey(tt.input); got != tt.want {
				t.Errorf("maskKey(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatExpiry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		exp      int64
		contains string
	}{
		{"zero", 0, "none"},
		{"future", time.Now().Add(30 * 24 * time.Hour).Unix(), "days remaining"},
		{"past", time.Now().Add(-30 * 24 * time.Hour).Unix(), "expired"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := formatExpiry(tt.exp)
			if tt.contains != "" && !strings.Contains(got, tt.contains) {
				t.Errorf("formatExpiry(%d) = %q, want to contain %q", tt.exp, got, tt.contains)
			}
		})
	}
}

func TestLicenseCmd_Registration(t *testing.T) {
	t.Parallel()

	subs := licenseCmd.Commands()
	wantNames := map[string]bool{"set": false, "show": false, "remove": false}

	for _, sub := range subs {
		if _, ok := wantNames[sub.Name()]; ok {
			wantNames[sub.Name()] = true
		}
	}

	for name, found := range wantNames {
		if !found {
			t.Errorf("missing subcommand %q", name)
		}
	}
}
