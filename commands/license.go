package commands

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/klurvio/sukko-cli/client"
	clicontext "github.com/klurvio/sukko-cli/context"
	"github.com/spf13/cobra"
)

func init() {
	licenseCmd.AddCommand(licenseSetCmd, licenseShowCmd, licenseRemoveCmd, licensePushCmd)
	rootCmd.AddCommand(licenseCmd)
}

var licenseCmd = &cobra.Command{
	Use:   "license",
	Short: "Manage Sukko license key",
	Long:  "Store, view, and remove the Sukko license key from the CLI context.",
}

// licenseClaims represents the decoded payload of a license key.
type licenseClaims struct {
	Edition string `json:"edition"`
	Org     string `json:"org"`
	Exp     int64  `json:"exp"`
}

// decodeLicenseClaims splits a license key on ".", base64url-decodes the first
// segment (payload), and unmarshals the JSON claims. It does NOT verify the
// Ed25519 signature — the CLI doesn't have the public key.
func decodeLicenseClaims(key string) (*licenseClaims, error) {
	parts := strings.SplitN(key, ".", 2)
	if len(parts) != 2 {
		return nil, errors.New("invalid license key format: expected payload.signature")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("decode license payload: %w", err)
	}

	// Validate signature segment is decodable (format check only)
	if _, err := base64.RawURLEncoding.DecodeString(parts[1]); err != nil {
		return nil, fmt.Errorf("decode license signature: %w", err)
	}

	var claims licenseClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("unmarshal license claims: %w", err)
	}

	return &claims, nil
}

// formatExpiry returns a human-readable expiry string with days remaining/since.
func formatExpiry(exp int64) string {
	if exp == 0 {
		return "none"
	}
	t := time.Unix(exp, 0)
	remaining := time.Until(t)
	if remaining > 0 {
		return fmt.Sprintf("%s (%d days remaining)", t.Format("2006-01-02"), int(remaining.Hours()/24))
	}
	return fmt.Sprintf("%s (expired %d days ago)", t.Format("2006-01-02"), int(-remaining.Hours()/24))
}

// --- sukko license set ---

var licenseSetCmd = &cobra.Command{
	Use:   "set [key]",
	Short: "Store a license key in the active context",
	Long: `Store a Sukko license key in the CLI context. The key is encrypted at rest.

If no key argument is provided, you will be prompted for input to avoid
the key appearing in shell history.`,
	RunE: runLicenseSet,
}

func runLicenseSet(cmd *cobra.Command, args []string) error {
	var key string
	if len(args) > 0 {
		key = args[0]
	} else {
		// Prompt to avoid shell history (FR-025)
		fmt.Fprint(cmd.OutOrStdout(), "License key: ")
		if _, err := fmt.Scanln(&key); err != nil {
			return fmt.Errorf("read license key: %w", err)
		}
	}

	key = strings.TrimSpace(key)
	if key == "" {
		return errors.New("license key is required")
	}

	// Validate format (FR-020)
	claims, err := decodeLicenseClaims(key)
	if err != nil {
		return fmt.Errorf("validate license key: %w", err)
	}

	// Warn if expired (FR-024)
	if claims.Exp > 0 && time.Unix(claims.Exp, 0).Before(time.Now()) {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"Warning: this license appears to be expired (%s). The platform will run as Community edition.\n",
			time.Unix(claims.Exp, 0).Format("2006-01-02"))
	}

	// Store encrypted in context
	store, ctx, err := requireActiveContext()
	if err != nil {
		return err
	}

	enc, err := store.EncryptSecret(key)
	if err != nil {
		return fmt.Errorf("encrypt license key: %w", err)
	}

	ctx.LicenseKeyEnc = enc
	if err := store.Add(ctx); err != nil {
		return fmt.Errorf("save context: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "License stored. Edition: %s, Org: %s, Expires: %s\n",
		capitalizeEdition(claims.Edition), claims.Org, formatExpiry(claims.Exp))

	return nil
}

// --- sukko license show ---

var licenseShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Display the stored license key and decoded claims",
	RunE:  runLicenseShow,
}

func runLicenseShow(cmd *cobra.Command, _ []string) error {
	store, ctx, err := requireActiveContext()
	if err != nil {
		return err
	}

	key, err := ctx.LicenseKey(store.Key())
	if err != nil {
		return fmt.Errorf("decrypt license key: %w", err)
	}
	if key == "" {
		fmt.Fprintln(cmd.OutOrStdout(), "No license key stored in the active context.")
		return nil
	}

	// Mask key (FR-021)
	masked := maskKey(key)
	fmt.Fprintf(cmd.OutOrStdout(), "Key:         %s\n", masked)

	claims, err := decodeLicenseClaims(key)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not decode claims: %v\n", err)
		return nil
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Edition:     %s\n", capitalizeEdition(claims.Edition))
	if claims.Org != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Org:         %s\n", claims.Org)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Expires:     %s\n", formatExpiry(claims.Exp))

	return nil
}

func maskKey(key string) string {
	if len(key) <= 12 {
		return "***"
	}
	return key[:8] + "..." + key[len(key)-4:]
}

// --- sukko license remove ---

var licenseRemoveCmd = &cobra.Command{
	Use:   "remove",
	Short: "Remove the stored license key from the active context",
	RunE:  runLicenseRemove,
}

func runLicenseRemove(cmd *cobra.Command, _ []string) error {
	store, ctx, err := requireActiveContext()
	if err != nil {
		return err
	}

	if ctx.LicenseKeyEnc == "" {
		fmt.Fprintln(cmd.OutOrStdout(), "No license key stored.")
		return nil
	}

	ctx.LicenseKeyEnc = ""
	if err := store.Add(ctx); err != nil {
		return fmt.Errorf("save context: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "License key removed from context %q.\n", ctx.Name)
	return nil
}

// --- sukko license push ---

var licensePushCmd = &cobra.Command{
	Use:   "push [key]",
	Short: "Push a license key to a running deployment",
	Long: `Push a Sukko license key to a running provisioning service via
POST /api/v1/license. The license is applied immediately without
restarting services.

If no key argument is provided, you will be prompted for input to avoid
the key appearing in shell history.

This command does NOT modify the local context — use 'sukko license set'
to persist the key for future 'sukko up' invocations.`,
	RunE: runLicensePush,
}

func runLicensePush(cmd *cobra.Command, args []string) error {
	// 1. Get key from args or prompt (matching license set pattern)
	var key string
	if len(args) > 0 {
		key = args[0]
	} else {
		fmt.Fprint(cmd.OutOrStdout(), "License key: ")
		if _, err := fmt.Scanln(&key); err != nil {
			return fmt.Errorf("read license key: %w", err)
		}
	}

	key = strings.TrimSpace(key)
	if key == "" {
		return errors.New("license key is required")
	}

	// 2. Local format validation
	claims, err := decodeLicenseClaims(key)
	if err != nil {
		return fmt.Errorf("validate license key: %w", err)
	}

	// 3. Warn if expired (don't fail — server is authoritative)
	if claims.Exp > 0 && time.Unix(claims.Exp, 0).Before(time.Now()) {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"Warning: this license appears to be expired (%s). The server's validation is authoritative.\n",
			time.Unix(claims.Exp, 0).Format("2006-01-02"))
	}

	// 4. Create admin-authenticated client (fails fast if no keypair)
	c, err := newClient()
	if err != nil {
		return err
	}

	// 5. Push license to provisioning
	resp, err := c.PushLicense(cmd.Context(), key)
	if err != nil {
		if errors.Is(err, client.ErrAPIUnauthorized) {
			return errors.New("admin authentication failed — ensure your admin key is registered via 'sukko auth register' or ADMIN_BOOTSTRAP_KEY")
		}
		if errors.Is(err, client.ErrAPIRateLimited) {
			return fmt.Errorf("license endpoint is rate-limited — %w", err)
		}
		return fmt.Errorf("push license: %w", err)
	}

	// 6. Report success
	fmt.Fprintf(cmd.OutOrStdout(), "License applied. Edition: %s, Org: %s, Expires: %s\n",
		capitalizeEdition(resp.Edition), resp.Org, formatExpiry(parseExpiry(resp.ExpiresAt)))

	return nil
}

// parseExpiry converts an RFC3339 expiry string to a Unix timestamp.
// Returns 0 if parsing fails (formatExpiry handles 0 as "none").
func parseExpiry(s string) int64 {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return 0
	}
	return t.Unix()
}

// requireActiveContext returns the store and active context, or an error.
func requireActiveContext() (*clicontext.Store, *clicontext.Context, error) {
	if resolvedStore == nil || resolvedCtx == nil {
		return nil, nil, errors.New("no active context — run 'sukko init' first")
	}
	return resolvedStore, resolvedCtx, nil
}
