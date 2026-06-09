package commands

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRunTest_BackendRejectedByCapabilities verifies that --message-backend=nats
// (or any backend not advertised by the tester) is rejected client-side before
// the start-test request is sent.
func TestRunTest_BackendRejectedByCapabilities(t *testing.T) {
	// No t.Parallel() — modifies package-level flag vars testTesterURL and testMessageBackend.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/capabilities" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"backends":       []string{"direct", "kafka"},
				"test_types":     []any{},
				"suites":         []any{},
				"context_fields": []any{},
			})
			return
		}
		// start-test endpoint must NOT be reached — capabilities check should abort first
		t.Errorf("unexpected request to %s — should have been aborted before reaching start-test", r.URL.Path)
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer srv.Close()

	orig := testTesterURL
	testTesterURL = srv.URL
	defer func() { testTesterURL = orig }()

	origBackend := testMessageBackend
	testMessageBackend = "nats"
	defer func() { testMessageBackend = origBackend }()

	err := runTest(nil, "smoke", nil)
	if err == nil {
		t.Fatal("expected error for unsupported message_backend=nats, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported message backend") {
		t.Errorf("error = %q, want it to contain 'unsupported message backend'", err.Error())
	}
}

// TestRunTest_SupportedBackendPassesCapabilities verifies that a supported backend
// (kafka) passes the capabilities check and the start-test request is sent.
func TestRunTest_SupportedBackendPassesCapabilities(t *testing.T) {
	// No t.Parallel() — modifies package-level flag vars testTesterURL and testMessageBackend.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/capabilities":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"backends":       []string{"direct", "kafka"},
				"test_types":     []any{},
				"suites":         []any{},
				"context_fields": []any{},
			})
		case "/api/v1/tests":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":     "abc12345",
				"status": "running",
				"config": map[string]any{},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	orig := testTesterURL
	testTesterURL = srv.URL
	defer func() { testTesterURL = orig }()

	origBackend := testMessageBackend
	testMessageBackend = "kafka"
	defer func() { testMessageBackend = origBackend }()

	err := runTest(nil, "smoke", nil)
	if err != nil {
		t.Errorf("unexpected error for supported backend kafka: %v", err)
	}
}
