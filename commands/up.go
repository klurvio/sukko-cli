package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/klurvio/sukko-cli/compose"
	"github.com/spf13/cobra"
)

const healthTimeout = 120 * time.Second

// Docker Compose internal service addresses — used by buildComposeConfig
// to wire services together inside the compose network.
const (
	composeDatabaseURL    = "postgres://sukko:sukko@postgres:5432/sukko_provisioning?sslmode=disable" //nolint:gosec // G101: not a credential — Docker Compose internal connection string with well-known dev defaults
	composeValkeyAddr     = "valkey:6379"
	composeKafkaBroker    = "kafka:9092"
	composeRedpandaBroker = "redpanda:9092"
	composeNATSURL        = "nats://nats:4222"
)

var pullImages bool

func init() {
	upCmd.Flags().BoolVar(&pullImages, "pull", false, "Always pull latest images before starting")
	rootCmd.AddCommand(upCmd)
}

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Start the local development environment",
	Long: `Start the local development environment using Docker Compose.

Reads .sukko/config.json (created by 'sukko init'), activates the selected
infrastructure profiles, waits for all services to become healthy, and
provisions a default tenant for immediate use.`,
	RunE: runUp,
}

func runUp(cmd *cobra.Command, _ []string) error {
	// Read project config
	configPath := filepath.Join(".", sukkoConfigDir, sukkoConfigFile)
	data, err := os.ReadFile(configPath) //nolint:gosec // G304: path derived from fixed constant, not user input
	if err != nil {
		return fmt.Errorf("read config (run 'sukko init' first): %w", err)
	}

	var cfg ProjectConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	// Verify credentials are available before starting services
	if resolvedCtx == nil {
		return errors.New("no active context found — run 'sukko init' first")
	}
	if _, _, err := resolveProvisioningConfig(); err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "Hint: run 'sukko init' to set up admin keypair")
		return fmt.Errorf("resolve credentials: %w", err)
	}

	// Build profiles and env overrides from selections
	profiles, envOverrides := buildComposeConfig(cfg)

	// best-effort: license key passthrough is optional; decrypt failure is non-fatal
	if resolvedCtx != nil && resolvedStore != nil {
		if lk, err := resolvedCtx.LicenseKey(resolvedStore.Key()); err == nil && lk != "" {
			envOverrides["SUKKO_LICENSE_KEY"] = lk
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Starting Sukko (%s + %s + %s)...\n",
		cfg.Database, cfg.Broadcast, cfg.MessageBackend)

	// Write embedded compose file to .sukko/
	if err := compose.WriteComposeFile(composeFilePath()); err != nil {
		return fmt.Errorf("write compose file: %w", err)
	}

	// Start Docker Compose
	mgr, err := compose.NewManager(".", composeFilePath())
	if err != nil {
		return fmt.Errorf("create compose manager: %w", err)
	}
	if err := mgr.Up(cmd.Context(), profiles, envOverrides, pullImages); err != nil {
		return fmt.Errorf("start services: %w", err)
	}

	// Wait for health — resolve URLs from context.
	fmt.Fprintln(cmd.OutOrStdout(), "\nWaiting for services to become healthy...")

	provURL := defaultAPIURL
	gwHTTP := defaultGatewayHTTP
	serverURL := defaultServerURL
	testerURL := defaultTesterURL

	if resolvedCtx != nil {
		if resolvedCtx.ProvisioningURL != "" {
			provURL = resolvedCtx.ProvisioningURL
		}
		if resolvedCtx.GatewayURL != "" {
			gwHTTP = wsToHTTP(resolvedCtx.GatewayURL)
		}
		if resolvedCtx.TesterURL != "" {
			testerURL = resolvedCtx.TesterURL
		}
	}

	targets := []compose.HealthTarget{
		{Name: "provisioning", URL: provURL + "/health"},
		{Name: "ws-gateway", URL: gwHTTP + "/health"},
		{Name: "ws-server", URL: serverURL + "/health"},
		{Name: "sukko-tester", URL: testerURL + "/health"},
	}

	if err := compose.WaitForHealth(cmd.Context(), cmd.OutOrStdout(), targets, healthTimeout); err != nil {
		return fmt.Errorf("health check: %w", err)
	}

	// Observability services — warn on error, don't fail (NFR-002)
	if cfg.Observability {
		obsTargets := []compose.HealthTarget{
			{Name: "grafana", URL: "http://localhost:3030/api/health"},
			{Name: "prometheus", URL: "http://localhost:9091/-/healthy"},
		}
		if err := compose.WaitForHealth(cmd.Context(), cmd.OutOrStdout(), obsTargets, healthTimeout); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: observability services not healthy: %v\n", err)
		}
	}

	// Provision default tenant
	fmt.Fprintln(cmd.OutOrStdout(), "\nProvisioning default tenant...")
	if err := provisionDefaultTenant(cmd); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: default tenant provisioning failed: %v\n", err)
		fmt.Fprintln(cmd.ErrOrStderr(), "  Services are running. Create a tenant manually with 'sukko tenant create'.")
	}

	fmt.Fprintln(cmd.OutOrStdout(), "\nSukko is ready! Try:")
	fmt.Fprintln(cmd.OutOrStdout(), "  sukko status")
	fmt.Fprintln(cmd.OutOrStdout(), "  sukko tenant list")

	if cfg.Observability {
		fmt.Fprintln(cmd.OutOrStdout(), "\nObservability:")
		fmt.Fprintln(cmd.OutOrStdout(), "  Grafana:      http://localhost:3030")
		fmt.Fprintln(cmd.OutOrStdout(), "  Prometheus:   http://localhost:9091")
		fmt.Fprintln(cmd.OutOrStdout(), "  AlertManager: http://localhost:9093")
	}

	return nil
}

func buildComposeConfig(cfg ProjectConfig) (profiles []string, envOverrides map[string]string) {
	envOverrides = map[string]string{}

	// Postgres is the only supported database — always activate
	profiles = append(profiles, "postgres")
	envOverrides["DATABASE_DRIVER"] = "postgres"
	envOverrides["DATABASE_URL"] = composeDatabaseURL

	if cfg.Broadcast == "valkey" {
		profiles = append(profiles, "valkey")
		envOverrides["BROADCAST_TYPE"] = "valkey"
		envOverrides["VALKEY_ADDRS"] = composeValkeyAddr
	}

	switch cfg.MessageBackend {
	case "kafka":
		profiles = append(profiles, "kafka")
		envOverrides["MESSAGE_BACKEND"] = "kafka"
		envOverrides["KAFKA_BROKERS"] = composeKafkaBroker
	case "redpanda":
		profiles = append(profiles, "redpanda")
		envOverrides["MESSAGE_BACKEND"] = "kafka"
		envOverrides["KAFKA_BROKERS"] = composeRedpandaBroker
	case "nats":
		envOverrides["MESSAGE_BACKEND"] = "nats"
		envOverrides["NATS_JETSTREAM_URLS"] = composeNATSURL
	}

	if cfg.Observability {
		profiles = append(profiles, "observability")
		if cfg.Tracing {
			envOverrides["OTEL_TRACING_ENABLED"] = "true"
		}
		if cfg.Profiling {
			envOverrides["PPROF_ENABLED"] = "true"
			envOverrides["PYROSCOPE_ENABLED"] = "true"
		}
	}

	return profiles, envOverrides
}

func provisionDefaultTenant(cmd *cobra.Command) error {
	c, err := newClient()
	if err != nil {
		return fmt.Errorf("create client: %w", err)
	}

	ctx := cmd.Context()

	// Create default tenant (ignore conflict — may already exist)
	_, err = c.CreateTenant(ctx, map[string]any{
		"id":            "demo",
		"name":          "Demo",
		"consumer_type": "shared",
	})
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "  Tenant 'demo': %v (may already exist)\n", err)
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), "  Tenant 'demo': created")
	}

	// Set catch-all routing rules
	_, err = c.SetRoutingRules(ctx, "demo", map[string]any{
		"rules": []map[string]any{
			{"pattern": "*.*", "topic_suffix": "default"},
		},
	})
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "  Routing rules: %v\n", err)
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), "  Routing rules: set (catch-all)")
	}

	// Set all-public channel rules
	_, err = c.SetChannelRules(ctx, "demo", map[string]any{
		"public_patterns": []string{"*"},
	})
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "  Channel rules: %v\n", err)
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), "  Channel rules: set (all public)")
	}

	return nil
}
