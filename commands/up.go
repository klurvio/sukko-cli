package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/klurvio/sukko-cli/client"
	"github.com/klurvio/sukko-cli/compose"
	"github.com/spf13/cobra"
)

const healthTimeout = 120 * time.Second

// Docker Compose internal service addresses — used by buildComposeConfig
// to wire services together inside the compose network.
const (
	composeDatabaseURL    = "postgres://sukko:sukko@postgres:5432/sukko_provisioning?sslmode=disable" //nolint:gosec // G101: not a credential — Docker Compose internal connection string with well-known dev defaults
	composeValkeyAddr     = "valkey:6379"
	composeRedpandaBroker = "redpanda:9092"
)

// Message backend selectors accepted in project config (sukko init). Both "kafka" and
// "redpanda" select the local Redpanda broker (Kafka-wire-compatible); see isKafkaFamilyBackend.
const (
	backendKafka    = "kafka"
	backendRedpanda = "redpanda"
)

// isKafkaFamilyBackend reports whether the message backend selects the Kafka/Redpanda
// broker (as opposed to the default "direct" backend). Both aliases are Pro/Enterprise-gated.
func isKafkaFamilyBackend(backend string) bool {
	return backend == backendKafka || backend == backendRedpanda
}

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

	if err := validateProjectConfig(cfg); err != nil {
		return err
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

	// Credentials encryption key (required by provisioning for encrypting secrets in DB)
	if cfg.CredentialsEncKey != "" {
		envOverrides["CREDENTIALS_ENCRYPTION_KEY"] = cfg.CredentialsEncKey
	}

	// Admin bootstrap key — provisioning registers this on first startup for JWT auth
	adminPubKey := loadAdminPublicKey()
	if adminPubKey != "" {
		envOverrides["ADMIN_BOOTSTRAP_KEY"] = adminPubKey
	}

	// best-effort: license key passthrough is optional; decrypt failure is non-fatal
	if resolvedCtx != nil && resolvedStore != nil {
		if lk, err := resolvedCtx.LicenseKey(resolvedStore.Key()); err == nil && lk != "" {
			envOverrides["SUKKO_LICENSE_KEY"] = lk
		}
	}

	// Kafka mode is Pro/Enterprise-gated — ws-server rejects MESSAGE_BACKEND=kafka on
	// Community at startup. Fail fast with a clear message instead of a container
	// crash-loop on a deep FeatureError. The server startup gate remains authoritative.
	//
	// The check must mirror what compose's ${SUKKO_LICENSE_KEY:-} interpolation actually
	// resolves (see resolveEffectiveLicenseKey).
	if err := requireLicenseForKafka(cfg.MessageBackend, resolveEffectiveLicenseKey(envOverrides)); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Starting Sukko (postgres + %s + %s)...\n",
		cfg.Broadcast, cfg.MessageBackend)

	// Write embedded compose file to .sukko/
	if err := compose.WriteComposeFile(composeFilePath()); err != nil {
		return fmt.Errorf("write compose file: %w", err)
	}

	// Start Docker Compose
	fmt.Fprintln(cmd.OutOrStdout(), "\nStarting containers...")
	mgr, err := compose.NewManager(".", composeFilePath())
	if err != nil {
		return fmt.Errorf("create compose manager: %w", err)
	}
	if err := mgr.Up(cmd.Context(), profiles, envOverrides, pullImages); err != nil {
		return fmt.Errorf("start services: %w", err)
	}

	// Wait for core services to become healthy (delegates to docker compose ps)
	fmt.Fprintln(cmd.OutOrStdout(), "\nWaiting for services...")

	if err := mgr.WaitForHealth(cmd.Context(), cmd.OutOrStdout(), []string{"provisioning", "ws-gateway", "ws-server", "sukko-tester"}, healthTimeout); err != nil {
		return fmt.Errorf("health check: %w", err)
	}

	// Observability services — warn on error, don't fail
	if cfg.Observability {
		if err := mgr.WaitForHealth(cmd.Context(), cmd.OutOrStdout(), []string{"grafana", "prometheus"}, healthTimeout); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: observability services not healthy: %v\n", err)
		}
	}

	// Provision default tenant
	fmt.Fprintln(cmd.OutOrStdout(), "\nProvisioning default tenant...")
	if err := provisionDefaultTenant(cmd); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: default tenant provisioning failed: %v\n", err)
		fmt.Fprintln(cmd.ErrOrStderr(), "  Services are running. Create a tenant manually with 'sukko tenant create'.")
	}

	// Push-service reconciliation — start/stop based on final edition
	reconcilePushService(cmd, mgr, cfg)

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

// requireLicenseForKafka returns an error when a kafka-family message backend is selected
// without a configured license key. Kafka mode is Pro/Enterprise-gated (ws-server rejects it
// at startup), so this gives a clear operator-facing failure before `compose up` rather than a
// container crash-loop. The server startup gate remains the authoritative enforcement point.
func requireLicenseForKafka(backend, licenseKey string) error {
	if isKafkaFamilyBackend(backend) && licenseKey == "" {
		return fmt.Errorf("message backend %q requires a Pro/Enterprise license, but none is configured — set one with 'sukko license set <token>' or switch to direct mode", backend)
	}
	return nil
}

// resolveEffectiveLicenseKey returns the license key that compose's ${SUKKO_LICENSE_KEY:-}
// interpolation will actually see: the context-store key (already placed in envOverrides) takes
// precedence, falling back to a shell-exported SUKKO_LICENSE_KEY (inherited by the compose
// subprocess via os.Environ() in compose.Manager.Up). Keeping this in sync with the real
// resolution prevents requireLicenseForKafka from false-positiving on the documented
// `export SUKKO_LICENSE_KEY` workflow.
func resolveEffectiveLicenseKey(envOverrides map[string]string) string {
	if lk := envOverrides["SUKKO_LICENSE_KEY"]; lk != "" {
		return lk
	}
	return os.Getenv("SUKKO_LICENSE_KEY")
}

// validateProjectConfig rejects stale project config values that are no longer supported.
func validateProjectConfig(cfg ProjectConfig) error {
	if cfg.Broadcast == "nats" {
		return errors.New(`broadcast "nats" is no longer supported: update .sukko/config.json to remove the broadcast field or set it to "valkey"`)
	}
	return nil
}

func buildComposeConfig(cfg ProjectConfig) (profiles []string, envOverrides map[string]string) {
	envOverrides = map[string]string{}

	// Postgres is the only supported database — always activate
	profiles = append(profiles, "postgres")
	envOverrides["DATABASE_DRIVER"] = "postgres"
	envOverrides["DATABASE_URL"] = composeDatabaseURL

	// Valkey is always the broadcast bus; wire its compose-internal address.
	// When cfg.MessageBackend is empty (direct mode — the Go default), no MESSAGE_BACKEND
	// env var is injected and the Go envDefault ("direct") takes effect natively.
	envOverrides["VALKEY_ADDRS"] = composeValkeyAddr

	// Kafka mode: "kafka" and "redpanda" both select the local Redpanda broker
	// (Kafka-wire-compatible) under the "kafka" compose profile — the only profile the
	// redpanda service declares. (Previously "kafka" wired the nonexistent host kafka:9092
	// and "redpanda" appended a "redpanda" profile no service defines — both broken.)
	switch cfg.MessageBackend {
	case backendKafka, backendRedpanda:
		profiles = append(profiles, "kafka")
		envOverrides["MESSAGE_BACKEND"] = "kafka"
		envOverrides["KAFKA_BROKERS"] = composeRedpandaBroker
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
	_, err = c.SetRoutingRules(ctx, "demo", defaultCatchAllRules())
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "  Routing rules: %v\n", err)
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), "  Routing rules: set (catch-all)")
	}

	// Seed channel rules. Channel authorization is provisioning-only — a tenant
	// with no rules is denied every subscribe and publish — so the demo tenant
	// must be seeded to be usable out of the box.
	_, err = c.SetChannelRules(ctx, "demo", defaultChannelRules())
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "  Channel rules: %v\n", err)
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), "  Channel rules: set (all channels open, subscribe + publish — override with 'sukko rules channels set')")
	}

	return nil
}

// defaultChannelRules returns the channel-rules request body seeded for the
// demo tenant: all channels open for subscribe AND publish. Field names match
// the provisioning SetChannelRulesRequest schema (public/default/publish_public).
// The previous "public_patterns" key was unknown to the API and was silently
// dropped, saving EMPTY rules — a deny-all tenant. Extracted so up_test.go can
// assert the exact schema without network calls.
func defaultChannelRules() map[string]any {
	return map[string]any{
		"public":         []string{"*"},
		"default":        []string{"*"},
		"publish_public": []string{"*"},
	}
}

// defaultCatchAllRules returns the routing rules request body for the catch-all default rule.
// Extracted so up_test.go can assert the correct format without network calls.
func defaultCatchAllRules() map[string]any {
	return map[string]any{
		"rules": []map[string]any{
			{"pattern": "**", "topics": []string{"default"}, "priority": 100},
		},
	}
}

// loadAdminPublicKey reads the admin public key (raw base64) from the context directory.
// Returns empty string if no keypair exists (user hasn't run 'sukko auth keygen').
func loadAdminPublicKey() string {
	keyPath := resolveAdminKeyPath()
	if keyPath == "" {
		return ""
	}
	pubPath := strings.TrimSuffix(keyPath, filepath.Ext(keyPath)) + ".pub"
	data, err := os.ReadFile(pubPath) //nolint:gosec // G304: path derived from context directory, not user input
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

const pushServiceTimeout = 30 * time.Second

// reconcilePushService starts or stops push-service based on the final edition.
// Uses findLocalContext for license key and provisioning URL (FR-006).
// Precondition: runUp has already verified resolvedCtx != nil, which guarantees resolvedStore != nil.
func reconcilePushService(cmd *cobra.Command, mgr *compose.Manager, cfg ProjectConfig) {
	localCtx, err := resolvedStore.FindLocalContext()
	if err != nil || localCtx == nil {
		return // no local context — skip reconciliation
	}

	// Decrypt license key from local context
	licenseKey, err := localCtx.LicenseKey(resolvedStore.Key())
	if err != nil || licenseKey == "" {
		return // no license key — skip reconciliation
	}

	// Create client targeting local provisioning
	localSigner, err := loadAdminSigner()
	if err != nil {
		return // no admin keypair — skip silently
	}

	c, err := client.New(client.Config{
		BaseURL: localCtx.ProvisioningURL,
		Signer:  localSigner,
	})
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not create provisioning client: %v — push-service not reconciled.\n", err)
		return
	}

	// Push license explicitly (idempotent — provisioning may have auto-loaded it)
	resp, err := c.PushLicense(cmd.Context(), licenseKey)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: license push failed: %v — push-service not started. Run 'sukko license push <key>' to retry.\n", err)
		return
	}

	edition := resp.Edition
	compatibleBackend := isKafkaFamilyBackend(cfg.MessageBackend)

	if edition == "enterprise" && compatibleBackend {
		if !pushServiceHealthy(cmd.Context(), mgr) {
			fmt.Fprintln(cmd.OutOrStdout(), "\nStarting push-service (Enterprise)...")
			if err := mgr.StartService(cmd.Context(), cmd.OutOrStdout(), "push-service", []string{"enterprise"}, pushServiceTimeout); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Warning: push-service failed to start: %v\n", err)
			}
		}
	} else if edition != "enterprise" {
		if pushServiceRunning(cmd.Context(), mgr) {
			fmt.Fprintln(cmd.OutOrStdout(), "\nStopping push-service (no longer Enterprise)...")
			if err := mgr.StopService(cmd.Context(), "push-service"); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to stop push-service: %v\n", err)
			}
		}
	}
}

// pushServiceHealthy returns true if push-service reports Health=="healthy" via docker compose ps.
func pushServiceHealthy(ctx context.Context, mgr *compose.Manager) bool {
	statuses, err := mgr.Status(ctx)
	if err != nil {
		return false
	}
	for _, s := range statuses {
		if s.Service == "push-service" && s.Health == "healthy" {
			return true
		}
	}
	return false
}

// pushServiceRunning returns true if push-service has a running container.
func pushServiceRunning(ctx context.Context, mgr *compose.Manager) bool {
	statuses, err := mgr.Status(ctx)
	if err != nil {
		return false
	}
	for _, s := range statuses {
		if s.Service == "push-service" && s.State == "running" {
			return true
		}
	}
	return false
}
