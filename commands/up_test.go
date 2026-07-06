package commands

import (
	"slices"
	"strings"
	"testing"
)

func TestDefaultCatchAllRules(t *testing.T) {
	t.Parallel()

	body := defaultCatchAllRules()

	rulesRaw, ok := body["rules"]
	if !ok {
		t.Fatal("defaultCatchAllRules: missing 'rules' key")
	}

	rules, ok := rulesRaw.([]map[string]any)
	if !ok || len(rules) != 1 {
		t.Fatalf("defaultCatchAllRules: expected exactly 1 rule, got %v", rulesRaw)
	}

	rule := rules[0]

	// Assert topics array is present and correct.
	topicsRaw, ok := rule["topics"]
	if !ok {
		t.Fatal("rule missing 'topics' key")
	}
	topics, ok := topicsRaw.([]string)
	if !ok || len(topics) != 1 || topics[0] != "default" {
		t.Errorf("topics = %v, want [\"default\"]", topicsRaw)
	}

	// Assert priority is present and correct.
	priorityRaw, ok := rule["priority"]
	if !ok {
		t.Error("rule missing 'priority' key")
	} else if priorityRaw != 100 {
		t.Errorf("rule priority = %v, want 100", priorityRaw)
	}

	// Assert pattern is present and correct.
	if rule["pattern"] != "**" {
		t.Errorf("rule pattern = %v, want \"**\"", rule["pattern"])
	}

	// Assert topic_suffix is NOT present (old format must not appear).
	if _, ok := rule["topic_suffix"]; ok {
		t.Error("rule must not contain 'topic_suffix' key (stale format)")
	}
}

func TestValidateProjectConfig_NATSBroadcast(t *testing.T) {
	t.Parallel()
	err := validateProjectConfig(ProjectConfig{Broadcast: "nats"})
	if err == nil {
		t.Fatal("expected error for broadcast=nats, got nil")
	}
	if !strings.Contains(err.Error(), "nats") {
		t.Errorf("error %q missing 'nats' context", err.Error())
	}
}

func TestBuildComposeConfig_Observability(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		cfg           ProjectConfig
		wantProfile   bool
		wantTracing   bool
		wantProfiling bool
	}{
		{
			name:        "observability disabled — no profile or env vars",
			cfg:         ProjectConfig{Database: "sqlite", Broadcast: "valkey", MessageBackend: "direct"},
			wantProfile: false,
		},
		{
			name:        "observability enabled — adds profile",
			cfg:         ProjectConfig{Database: "sqlite", Broadcast: "valkey", MessageBackend: "direct", Observability: true},
			wantProfile: true,
		},
		{
			name:        "tracing enabled — sets OTEL_TRACING_ENABLED",
			cfg:         ProjectConfig{Observability: true, Tracing: true},
			wantProfile: true,
			wantTracing: true,
		},
		{
			name:          "profiling enabled — sets PPROF and PYROSCOPE",
			cfg:           ProjectConfig{Observability: true, Profiling: true},
			wantProfile:   true,
			wantProfiling: true,
		},
		{
			name:          "all enabled",
			cfg:           ProjectConfig{Observability: true, Tracing: true, Profiling: true},
			wantProfile:   true,
			wantTracing:   true,
			wantProfiling: true,
		},
		{
			name:        "tracing without observability — no effect",
			cfg:         ProjectConfig{Tracing: true},
			wantProfile: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			profiles, envOverrides := buildComposeConfig(tt.cfg)

			hasProfile := slices.Contains(profiles, "observability")
			if hasProfile != tt.wantProfile {
				t.Errorf("observability profile: got %v, want %v", hasProfile, tt.wantProfile)
			}

			if tt.wantTracing {
				if envOverrides["OTEL_TRACING_ENABLED"] != "true" {
					t.Error("expected OTEL_TRACING_ENABLED=true")
				}
			} else if _, ok := envOverrides["OTEL_TRACING_ENABLED"]; ok {
				t.Error("unexpected OTEL_TRACING_ENABLED")
			}

			if tt.wantProfiling {
				if envOverrides["PPROF_ENABLED"] != "true" {
					t.Error("expected PPROF_ENABLED=true")
				}
				if envOverrides["PYROSCOPE_ENABLED"] != "true" {
					t.Error("expected PYROSCOPE_ENABLED=true")
				}
			} else {
				if _, ok := envOverrides["PPROF_ENABLED"]; ok {
					t.Error("unexpected PPROF_ENABLED")
				}
			}
		})
	}
}

func TestBuildComposeConfig_MessageBackend(t *testing.T) {
	tests := []struct {
		name        string
		backend     string
		wantProfile bool // "kafka" profile present
		wantEnv     bool // MESSAGE_BACKEND/KAFKA_BROKERS overrides present
	}{
		{"default empty stays direct", "", false, false},
		{"explicit direct", "direct", false, false},
		{"kafka", "kafka", true, true},
		{"redpanda maps to kafka profile+broker", "redpanda", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profiles, env := buildComposeConfig(ProjectConfig{Database: "sqlite", Broadcast: "valkey", MessageBackend: tt.backend})
			hasKafka := slices.Contains(profiles, "kafka")
			if hasKafka != tt.wantProfile {
				t.Errorf("kafka profile = %v, want %v (profiles=%v)", hasKafka, tt.wantProfile, profiles)
			}
			if slices.Contains(profiles, "redpanda") {
				t.Errorf("no service declares a %q profile; got %v", "redpanda", profiles)
			}
			if tt.wantEnv {
				if env["MESSAGE_BACKEND"] != "kafka" {
					t.Errorf("MESSAGE_BACKEND = %q, want kafka", env["MESSAGE_BACKEND"])
				}
				if env["KAFKA_BROKERS"] != "redpanda:9092" {
					t.Errorf("KAFKA_BROKERS = %q, want redpanda:9092", env["KAFKA_BROKERS"])
				}
			} else {
				if _, ok := env["MESSAGE_BACKEND"]; ok {
					t.Errorf("MESSAGE_BACKEND override present for backend %q; want none (Go default direct)", tt.backend)
				}
			}
		})
	}
}

func TestRequireLicenseForKafka(t *testing.T) {
	tests := []struct {
		name       string
		backend    string
		licenseKey string
		wantErr    bool
	}{
		{"direct needs no license", "direct", "", false},
		{"empty backend needs no license", "", "", false},
		{"kafka without license errors", "kafka", "", true},
		{"redpanda without license errors", "redpanda", "", true},
		{"kafka with license ok", "kafka", "some-token", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := requireLicenseForKafka(tt.backend, tt.licenseKey)
			if (err != nil) != tt.wantErr {
				t.Errorf("requireLicenseForKafka(%q, present=%v) err = %v, wantErr %v", tt.backend, tt.licenseKey != "", err, tt.wantErr)
			}
			if tt.wantErr && err != nil && !strings.Contains(err.Error(), "Pro/Enterprise license") {
				t.Errorf("error %q should mention the license requirement", err.Error())
			}
		})
	}
}

func TestResolveEffectiveLicenseKey(t *testing.T) {
	// Not parallel: mutates the process-wide SUKKO_LICENSE_KEY env var via t.Setenv.
	tests := []struct {
		name    string
		envOver map[string]string
		shell   string // shell-exported SUKKO_LICENSE_KEY ("" = unset)
		want    string
	}{
		{"neither set", map[string]string{}, "", ""},
		{"context-store only", map[string]string{"SUKKO_LICENSE_KEY": "ctx-token"}, "", "ctx-token"},
		{"shell env only (regression: must be honored)", map[string]string{}, "shell-token", "shell-token"},
		{"context store wins over shell", map[string]string{"SUKKO_LICENSE_KEY": "ctx-token"}, "shell-token", "ctx-token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SUKKO_LICENSE_KEY", tt.shell)
			if got := resolveEffectiveLicenseKey(tt.envOver); got != tt.want {
				t.Errorf("resolveEffectiveLicenseKey(%v) with shell=%q = %q, want %q", tt.envOver, tt.shell, got, tt.want)
			}
		})
	}
}

// TestDefaultChannelRules asserts the seeded channel-rules body matches the
// provisioning SetChannelRulesRequest schema exactly. Regression guard: the
// previous body used the unknown key "public_patterns", which the API decoder
// silently dropped — saving EMPTY rules and leaving the demo tenant deny-all
// under provisioning-only channel authorization.
func TestDefaultChannelRules(t *testing.T) {
	t.Parallel()

	body := defaultChannelRules()

	for _, key := range []string{"public", "default", "publish_public"} {
		raw, ok := body[key]
		if !ok {
			t.Fatalf("defaultChannelRules: missing %q key", key)
		}
		patterns, ok := raw.([]string)
		if !ok || len(patterns) != 1 || patterns[0] != "*" {
			t.Errorf("defaultChannelRules[%q] = %v, want [\"*\"]", key, raw)
		}
	}

	// Regression: the old wrong key must never reappear — the API ignores
	// unknown fields, so it would silently seed a deny-all tenant again.
	if _, present := body["public_patterns"]; present {
		t.Error("defaultChannelRules must not use the unknown \"public_patterns\" key")
	}
	if len(body) != 3 {
		t.Errorf("defaultChannelRules has %d keys, want exactly 3 (public, default, publish_public): %v", len(body), body)
	}
}
