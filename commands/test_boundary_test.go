package commands

import (
	"strings"
	"testing"

	clicontext "github.com/klurvio/sukko-cli/context"
)

// These tests mutate package-level resolvedCtx/resolvedStore — no t.Parallel()
// on subtests to avoid data races on shared globals.

func TestIsLocalContext(t *testing.T) {
	tests := []struct {
		name      string
		ctx       *clicontext.Context
		wantLocal bool
	}{
		{
			name:      "localhost gateway",
			ctx:       &clicontext.Context{GatewayURL: "ws://localhost:3000", ProvisioningURL: "http://remote:8080"},
			wantLocal: true,
		},
		{
			name:      "127.0.0.1 provisioning",
			ctx:       &clicontext.Context{GatewayURL: "wss://remote:3000", ProvisioningURL: "http://127.0.0.1:8080"},
			wantLocal: true,
		},
		{
			name:      "remote URLs",
			ctx:       &clicontext.Context{GatewayURL: "wss://ws.example.com", ProvisioningURL: "https://api.example.com"},
			wantLocal: false,
		},
		{
			name:      "nil context",
			ctx:       nil,
			wantLocal: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origCtx := resolvedCtx
			resolvedCtx = tt.ctx
			defer func() { resolvedCtx = origCtx }()

			if got := isLocalContext(); got != tt.wantLocal {
				t.Errorf("isLocalContext() = %v, want %v", got, tt.wantLocal)
			}
		})
	}
}

func TestBuildTestContext(t *testing.T) {
	tests := []struct {
		name           string
		ctx            *clicontext.Context
		store          *clicontext.Store
		messageBackend string
		wantNil        bool
		wantErr        bool
		wantErrContain string
	}{
		{
			name:    "nil context returns nil",
			ctx:     nil,
			wantNil: true,
		},
		{
			name:    "localhost returns nil",
			ctx:     &clicontext.Context{GatewayURL: "ws://localhost:3000", ProvisioningURL: "http://localhost:8080"},
			wantNil: true,
		},
		{
			name:    "nil store returns nil",
			ctx:     &clicontext.Context{GatewayURL: "wss://ws.example.com", ProvisioningURL: "https://api.example.com"},
			store:   nil,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origCtx := resolvedCtx
			origStore := resolvedStore
			resolvedCtx = tt.ctx
			resolvedStore = tt.store
			defer func() {
				resolvedCtx = origCtx
				resolvedStore = origStore
			}()

			result, err := buildTestContext(tt.messageBackend)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				if tt.wantErrContain != "" && !strings.Contains(err.Error(), tt.wantErrContain) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.wantErrContain)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantNil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}
		})
	}
}
