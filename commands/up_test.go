package commands

import (
	"slices"
	"testing"
)

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
			cfg:         ProjectConfig{Database: "sqlite", Broadcast: "nats", MessageBackend: "direct"},
			wantProfile: false,
		},
		{
			name:        "observability enabled — adds profile",
			cfg:         ProjectConfig{Database: "sqlite", Broadcast: "nats", MessageBackend: "direct", Observability: true},
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
