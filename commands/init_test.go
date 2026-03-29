package commands

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProjectConfig_BackwardCompat(t *testing.T) {
	t.Parallel()

	// Existing config without observability fields should unmarshal to false
	oldJSON := `{"database":"sqlite","broadcast":"nats","message_backend":"direct"}`

	var cfg ProjectConfig
	if err := json.Unmarshal([]byte(oldJSON), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if cfg.Observability {
		t.Error("Observability should be false for old config")
	}
	if cfg.Tracing {
		t.Error("Tracing should be false for old config")
	}
	if cfg.Profiling {
		t.Error("Profiling should be false for old config")
	}
	if cfg.Database != "sqlite" {
		t.Errorf("Database = %q, want sqlite", cfg.Database)
	}
}

func TestProjectConfig_Roundtrip(t *testing.T) {
	t.Parallel()

	cfg := ProjectConfig{
		Database:       "postgres",
		Broadcast:      "nats",
		MessageBackend: "kafka",
		Observability:  true,
		Tracing:        true,
		Profiling:      false,
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got ProjectConfig
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got != cfg {
		t.Errorf("roundtrip mismatch: got %+v, want %+v", got, cfg)
	}
}

func TestProjectConfig_OmitEmpty(t *testing.T) {
	t.Parallel()

	// When observability fields are false, omitempty should omit them
	cfg := ProjectConfig{
		Database:       "sqlite",
		Broadcast:      "nats",
		MessageBackend: "direct",
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	jsonStr := string(data)
	for _, field := range []string{"observability", "tracing", "profiling"} {
		if strings.Contains(jsonStr, field) {
			t.Errorf("expected %q to be omitted from JSON, got: %s", field, jsonStr)
		}
	}
}
