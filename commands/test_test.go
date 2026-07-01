package commands

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"
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

// TestRunTest_SuitePassedInBody verifies that runTest propagates the extra map
// (including suite) to the start-test endpoint via maps.Copy.
func TestRunTest_SuitePassedInBody(t *testing.T) {
	// No t.Parallel() — modifies package-level testTesterURL; depends on testLoadSuite == "".
	origSuite := testLoadSuite
	testLoadSuite = ""
	defer func() { testLoadSuite = origSuite }()

	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/capabilities":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"backends":       []string{"direct"},
				"test_types":     []any{},
				"suites":         []any{},
				"context_fields": []any{},
			})
		case "/api/v1/tests":
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":     "t1",
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

	err := runTest(nil, "stress", map[string]any{"suite": "revocation", "connections": 3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotBody == nil {
		t.Fatal("no request body received at start-test endpoint")
	}
	if gotBody["suite"] != "revocation" {
		t.Errorf("body[suite] = %v, want %q", gotBody["suite"], "revocation")
	}
}

// TestLoadSuiteInjectedFromFlag verifies that the testLoadSuite package-level
// variable (bound to --suite by cobra) flows into the POST body when non-empty
// and is absent when empty. Covers both stress and soak RunE paths.
func TestLoadSuiteInjectedFromFlag(t *testing.T) {
	// No t.Parallel() — modifies package-level testTesterURL and testLoadSuite.
	tests := []struct {
		name    string
		cmd     *cobra.Command
		suite   string
		wantKey bool
	}{
		{"stress/suite set", testStressCmd, "revocation", true},
		{"stress/suite empty", testStressCmd, "", false},
		{"soak/suite set", testSoakCmd, "revocation", true},
		{"soak/suite empty", testSoakCmd, "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotBody map[string]any
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v1/capabilities":
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(map[string]any{
						"backends": []string{"direct"}, "test_types": []any{},
						"suites": []any{}, "context_fields": []any{},
					})
				case "/api/v1/tests":
					_ = json.NewDecoder(r.Body).Decode(&gotBody)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(map[string]any{
						"id": "t1", "status": "running", "config": map[string]any{},
					})
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()

			origURL := testTesterURL
			origSuite := testLoadSuite
			origCtx := tc.cmd.Context()
			testTesterURL = srv.URL
			testLoadSuite = tc.suite
			tc.cmd.SetContext(context.Background())
			defer func() {
				testTesterURL = origURL
				testLoadSuite = origSuite
				tc.cmd.SetContext(origCtx)
			}()
			err := tc.cmd.RunE(tc.cmd, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotBody == nil {
				t.Fatal("no request body received at start-test endpoint")
			}
			if tc.wantKey {
				if gotBody["suite"] != tc.suite {
					t.Errorf("body[suite] = %v, want %q", gotBody["suite"], tc.suite)
				}
			} else {
				if _, ok := gotBody["suite"]; ok {
					t.Errorf("body[suite] = %v, want key absent when --suite is empty", gotBody["suite"])
				}
			}
		})
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

// TestRunTest_HintTextNoStatusCommand verifies that when --follow is false the
// hint line printed to cmd.OutOrStdout() contains "--follow" and does NOT contain
// "status --id". Regression guard: the old hint referenced a non-existent
// `sukko test status --id` command.
func TestRunTest_HintTextNoStatusCommand(t *testing.T) {
	// No t.Parallel() — modifies package-level flag vars.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/tests" {
			t.Errorf("unexpected request to %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":     "t1",
			"status": "running",
			"config": map[string]any{},
		})
	}))
	defer srv.Close()

	origURL := testTesterURL
	testTesterURL = srv.URL
	defer func() { testTesterURL = origURL }()

	origBackend := testMessageBackend
	testMessageBackend = "" // skip capabilities check
	defer func() { testMessageBackend = origBackend }()

	origFollow := testFollow
	testFollow = false
	defer func() { testFollow = origFollow }()

	origOutput := output
	output = "" // ensure text path, not json
	defer func() { output = origOutput }()

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	var buf strings.Builder
	cmd.SetOut(&buf)

	if err := runTest(cmd, "smoke", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "--follow") {
		t.Errorf("hint text missing '--follow': %q", got)
	}
	if strings.Contains(got, "status --id") {
		t.Errorf("hint text must not reference 'status --id': %q", got)
	}
}
