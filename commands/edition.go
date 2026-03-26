package commands

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/klurvio/sukko-cli/client"
	"github.com/spf13/cobra"
)

func init() {
	editionCmd.AddCommand(editionCompareCmd)
	rootCmd.AddCommand(editionCmd)
}

var editionCmd = &cobra.Command{
	Use:   "edition",
	Short: "Show current edition, limits, and usage",
	Long: `Show the current Sukko edition (Community/Pro/Enterprise), license status,
hard limits, and live resource usage.

Fetches data from the provisioning service's /edition endpoint.
Falls back to locally stored license key if the platform is unreachable.`,
	RunE: runEdition,
}

// resolveProvisioningURL returns the provisioning URL without requiring a valid
// token. Unlike resolveClientConfig(), this never fails on corrupted tokens —
// the /edition endpoint requires no authentication.
func resolveProvisioningURL() string {
	if apiURL != "" {
		return apiURL
	}
	if resolvedCtx != nil && resolvedCtx.ProvisioningURL != "" {
		return resolvedCtx.ProvisioningURL
	}
	return defaultAPIURL
}

func runEdition(cmd *cobra.Command, _ []string) error {
	// TODO: add tester /api/v1/edition as first fallback when monorepo implements FR-008

	// Try provisioning API (no auth required)
	c, err := client.New(client.Config{
		BaseURL: resolveProvisioningURL(),
		Timeout: client.DefaultClientTimeout,
	})
	if err != nil {
		return fmt.Errorf("create edition client: %w", err)
	}

	resp, apiErr := c.GetEdition(cmd.Context())
	if apiErr == nil {
		if output == "json" {
			return printJSON(resp)
		}
		printEditionStatus(cmd, resp)
		return nil
	}

	// API unreachable — try local license key from context
	if resolvedCtx != nil && resolvedStore != nil && resolvedCtx.LicenseKeyEnc != "" {
		lk, decErr := resolvedCtx.LicenseKey(resolvedStore.Key())
		if decErr == nil && lk != "" {
			claims, claimErr := decodeLicenseClaims(lk)
			if claimErr == nil {
				if output == "json" {
					return printJSON(map[string]any{
						"edition":    claims.Edition,
						"org":        claims.Org,
						"expires_at": claims.Exp,
						"source":     "local_license",
					})
				}
				out := cmd.OutOrStdout()
				fmt.Fprintf(out, "Edition:     %s\n", capitalizeEdition(claims.Edition))
				if claims.Org != "" {
					fmt.Fprintf(out, "Org:         %s\n", claims.Org)
				}
				if claims.Exp > 0 {
					fmt.Fprintf(out, "Expires:     %s\n", formatExpiry(claims.Exp))
				}
				fmt.Fprintln(out, "\n(Platform not running — usage data unavailable)")
				return nil
			}
		}
		// License key exists but could not be used (decrypt or claims decode failed)
		return errors.New("no edition info available — platform unreachable and stored license key could not be decoded. Run 'sukko license set' to update or 'sukko up' to start the platform")
	}

	// Contextual error messages
	if resolvedCtx == nil {
		return errors.New("no edition info available — run 'sukko init' to set up your local context")
	}
	return errors.New("no edition info available — run 'sukko up' to start the platform or 'sukko license set' to store a license key")
}

func printEditionStatus(cmd *cobra.Command, resp *client.EditionResponse) {
	out := cmd.OutOrStdout()

	if resp.Expired {
		printExpiredEdition(out, resp)
		return
	}

	if resp.Org == "" && strings.EqualFold(resp.Edition, "community") {
		printCommunityEdition(out, resp)
		return
	}

	printActiveEdition(out, resp)
}

func printActiveEdition(out io.Writer, resp *client.EditionResponse) {
	fmt.Fprintf(out, "Edition:     %s%s%s\n", colorBold, capitalizeEdition(resp.Edition), colorReset)
	if resp.Org != "" {
		fmt.Fprintf(out, "Org:         %s\n", resp.Org)
	}
	if resp.ExpiresAt != "" {
		if t, err := time.Parse(time.RFC3339, resp.ExpiresAt); err == nil {
			days := int(time.Until(t).Hours() / 24)
			fmt.Fprintf(out, "Expires:     %s (%s%d days remaining%s)\n",
				t.Format("2006-01-02"), colorGreen, days, colorReset)
		}
	}

	fmt.Fprintln(out, "\nResource Usage:")
	printUsageTable(out, &resp.Limits, &resp.Usage)
}

func printExpiredEdition(out io.Writer, resp *client.EditionResponse) {
	fmt.Fprintf(out, "Edition:     %sCommunity%s (%sEXPIRED%s — was %s",
		colorRed, colorReset, colorYellow, colorReset, capitalizeEdition(resp.Edition))
	if resp.Org != "" {
		fmt.Fprintf(out, ", org: %s", resp.Org)
	}
	fmt.Fprintln(out, ")")

	if resp.ExpiresAt != "" {
		if t, err := time.Parse(time.RFC3339, resp.ExpiresAt); err == nil {
			days := int(-time.Until(t).Hours() / 24)
			fmt.Fprintf(out, "Expires:     %s (%sexpired %d days ago%s)\n",
				t.Format("2006-01-02"), colorRed, days, colorReset)
		}
	}

	fmt.Fprintln(out, "\nResource Limits:")
	printLimitsOnly(out, &resp.Limits)
}

func printCommunityEdition(out io.Writer, resp *client.EditionResponse) {
	fmt.Fprintln(out, "Edition:     Community (free)")
	fmt.Fprintln(out, "\nResource Limits:")
	printLimitsOnly(out, &resp.Limits)
}

func printLimitsOnly(out io.Writer, limits *client.EditionLimits) {
	printLimitRow(out, "Tenants", limits.MaxTenants)
	printLimitRow(out, "Connections", limits.MaxTotalConnections)
	printLimitRow(out, "Shards", limits.MaxShards)
}

func printLimitRow(out io.Writer, name string, limit int) {
	if limit == 0 {
		fmt.Fprintf(out, "  %-16s Unlimited\n", name+":")
		return
	}
	fmt.Fprintf(out, "  %-16s %d\n", name+":", limit)
}

func printUsageTable(out io.Writer, limits *client.EditionLimits, usage *client.EditionUsage) {
	type row struct {
		name    string
		current *int
		max     int
	}

	rows := []row{
		{"Tenants", usage.Tenants, limits.MaxTenants},
		{"Connections", usage.Connections, limits.MaxTotalConnections},
		{"Shards", usage.Shards, limits.MaxShards},
	}

	for _, r := range rows {
		maxStr := strconv.Itoa(r.max)
		if r.max == 0 {
			maxStr = "Unlimited"
		}

		if r.current == nil {
			fmt.Fprintf(out, "  %-16s \u2014 / %s\n", r.name+":", maxStr) // em dash for unavailable
			continue
		}

		if r.max == 0 {
			fmt.Fprintf(out, "  %-16s %d / %s\n", r.name+":", *r.current, maxStr)
			continue
		}

		pct := *r.current * 100 / r.max
		color := colorGreen
		if pct > 90 {
			color = colorRed
		} else if pct > 75 {
			color = colorYellow
		}
		fmt.Fprintf(out, "  %-16s %s%d%s / %s  (%s%d%%%s)\n",
			r.name+":", color, *r.current, colorReset, maxStr, color, pct, colorReset)
	}
}

func capitalizeEdition(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// --- sukko edition compare ---

var editionCompareCmd = &cobra.Command{
	Use:   "compare",
	Short: "Compare Community, Pro, and Enterprise editions",
	Long: `Display a comparison table of all three Sukko editions.

This command works offline — the comparison data is hardcoded from the
published edition matrix. No running services required.`,
	RunE: runEditionCompare,
}

var editionMatrix = []struct {
	dimension  string
	community  string
	pro        string
	enterprise string
}{
	{"Tenants", "3", "50", "Unlimited"},
	{"Total Connections", "500", "10,000", "Unlimited"},
	{"Shards", "1", "8", "Unlimited"},
	{"Topics/Tenant", "10", "50", "Unlimited"},
	{"Routing Rules/Tenant", "10", "100", "Unlimited"},
	{"", "", "", ""},
	{"Message Backend", "direct", "+ kafka/nats", "All"},
	{"Database", "sqlite", "+ postgres", "All"},
	{"Per-Tenant Isolation", "No", "Yes", "Yes"},
	{"Alerting", "No", "Yes", "Yes"},
	{"SSE Transport", "No", "Yes", "Yes"},
	{"Web Push", "No", "No", "Yes"},
	{"Audit Logging", "No", "No", "Yes"},
	{"Admin UI SSO", "No", "No", "Yes"},
}

func runEditionCompare(cmd *cobra.Command, _ []string) error {
	if output == "json" {
		return printJSON(comparisonData())
	}

	out := cmd.OutOrStdout()

	// Best-effort: detect current edition for highlighting (short timeout — UI decoration only)
	currentEdition := ""
	c, err := client.New(client.Config{
		BaseURL: resolveProvisioningURL(),
		Timeout: 2 * time.Second,
	})
	if err == nil {
		if resp, apiErr := c.GetEdition(cmd.Context()); apiErr == nil {
			currentEdition = strings.ToLower(resp.Edition)
		}
	}

	header := fmt.Sprintf("%s%-24s %-14s %-14s %-14s%s",
		colorBold, "Dimension", "Community", "Pro", "Enterprise", colorReset)
	fmt.Fprintln(out, header)
	fmt.Fprintln(out, strings.Repeat("\u2500", 66)) // ─

	for _, row := range editionMatrix {
		if row.dimension == "" {
			fmt.Fprintln(out)
			continue
		}
		fmt.Fprintf(out, "%-24s %-14s %-14s %-14s\n",
			row.dimension, row.community, row.pro, row.enterprise)
	}

	fmt.Fprintln(out)
	if currentEdition != "" {
		fmt.Fprintf(out, "Current: %s%s%s \u25C0\n", colorBold, capitalizeEdition(currentEdition), colorReset) // ◀
	}
	fmt.Fprintf(out, "Upgrade: %shttps://docs.sukko.dev/editions/upgrade%s\n", colorCyan, colorReset)

	return nil
}

func comparisonData() map[string]any {
	return map[string]any{
		"editions": []map[string]any{
			{
				"name": "community",
				"limits": map[string]any{
					"tenants": 3, "total_connections": 500, "shards": 1,
					"topics_per_tenant": 10, "routing_rules_per_tenant": 10,
				},
				"features": map[string]any{
					"message_backend": "direct", "database": "sqlite",
					"per_tenant_isolation": false, "alerting": false,
					"sse_transport": false, "web_push": false,
					"audit_logging": false, "admin_ui_sso": false,
				},
			},
			{
				"name": "pro",
				"limits": map[string]any{
					"tenants": 50, "total_connections": 10000, "shards": 8,
					"topics_per_tenant": 50, "routing_rules_per_tenant": 100,
				},
				"features": map[string]any{
					"message_backend": "direct, kafka, nats", "database": "sqlite, postgres",
					"per_tenant_isolation": true, "alerting": true,
					"sse_transport": true, "web_push": false,
					"audit_logging": false, "admin_ui_sso": false,
				},
			},
			{
				"name": "enterprise",
				"limits": map[string]any{
					"tenants": "unlimited", "total_connections": "unlimited", "shards": "unlimited",
					"topics_per_tenant": "unlimited", "routing_rules_per_tenant": "unlimited",
				},
				"features": map[string]any{
					"message_backend": "all", "database": "all",
					"per_tenant_isolation": true, "alerting": true,
					"sse_transport": true, "web_push": true,
					"audit_logging": true, "admin_ui_sso": true,
				},
			},
		},
		"upgrade_url": "https://docs.sukko.dev/editions/upgrade",
	}
}
