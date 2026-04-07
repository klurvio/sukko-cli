package commands

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var authKeyName string

func init() {
	authCmd.AddCommand(authKeygenCmd)
	authCmd.AddCommand(authRegisterCmd)
	authCmd.AddCommand(authRevokeCmd)
	authCmd.AddCommand(authListCmd)
	authRegisterCmd.Flags().StringVar(&authKeyName, "name", "", "Admin name (defaults to context name)")
	rootCmd.AddCommand(authCmd)
}

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage admin authentication keypairs",
	Long:  "Generate, register, revoke, and list admin Ed25519 keypairs for JWT-based authentication.",
}

var authKeygenCmd = &cobra.Command{
	Use:   "keygen",
	Short: "Generate an Ed25519 admin keypair",
	Long:  "Generates a new Ed25519 keypair and saves it to the active context directory.",
	RunE: func(cmd *cobra.Command, _ []string) error {
		dir := resolveKeypairDir()
		if dir == "" {
			return fmt.Errorf("no context directory available — run 'sukko init' first")
		}

		keyPath := filepath.Join(dir, "admin.key")
		pubPath := filepath.Join(dir, "admin.pub")

		// Check if keypair already exists
		if _, err := os.Stat(keyPath); err == nil {
			return fmt.Errorf("keypair already exists at %s — delete it first to regenerate", keyPath)
		}

		// Generate Ed25519 keypair
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return fmt.Errorf("generate keypair: %w", err)
		}

		// Ensure directory exists
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}

		// Save private key (PEM, PKCS8, 0600 permissions)
		privDER, err := x509.MarshalPKCS8PrivateKey(priv)
		if err != nil {
			return fmt.Errorf("marshal private key: %w", err)
		}
		privPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER})
		if err := os.WriteFile(keyPath, privPEM, 0o600); err != nil {
			return fmt.Errorf("write private key: %w", err)
		}

		// Save public key (raw base64 — the format ADMIN_BOOTSTRAP_KEY accepts)
		pubBase64 := base64.StdEncoding.EncodeToString(pub)
		if err := os.WriteFile(pubPath, []byte(pubBase64+"\n"), 0o644); err != nil {
			return fmt.Errorf("write public key: %w", err)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Keypair generated:\n  Private: %s\n  Public:  %s\n", keyPath, pubPath)
		fmt.Fprintf(cmd.OutOrStdout(), "\nFor Kubernetes bootstrap:\n  helm upgrade ... --set provisioning.adminBootstrapKey=\"%s\"\n", pubBase64)
		return nil
	},
}

var authRegisterCmd = &cobra.Command{
	Use:   "register",
	Short: "Register your public key with provisioning",
	Long:  "Sends your public key to the provisioning API to enable JWT-based admin authentication.",
	RunE: func(cmd *cobra.Command, _ []string) error {
		c, err := newClient()
		if err != nil {
			return err
		}

		// Read public key
		pubPath := filepath.Join(resolveKeypairDir(), "admin.pub")
		pubData, err := os.ReadFile(pubPath)
		if err != nil {
			return fmt.Errorf("read public key %s: %w (run 'sukko auth keygen' first)", pubPath, err)
		}

		// Decode base64 → raw key → PEM for API
		raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(pubData)))
		if err != nil {
			return fmt.Errorf("decode public key: %w", err)
		}
		pubKey := ed25519.PublicKey(raw)
		der, err := x509.MarshalPKIXPublicKey(pubKey)
		if err != nil {
			return fmt.Errorf("marshal public key: %w", err)
		}
		pemKey := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))

		name := authKeyName
		if name == "" && resolvedCtx != nil {
			name = resolvedCtx.Name
		}
		if name == "" {
			name = "admin"
		}

		result, err := c.RegisterAdminKey(cmd.Context(), name, "Ed25519", pemKey)
		if err != nil {
			return fmt.Errorf("register admin key: %w", err)
		}

		keyID, _ := result["key_id"].(string)
		fmt.Fprintf(cmd.OutOrStdout(), "Admin key registered: %s (name: %s)\n", keyID, name)

		// Save key ID for JWT signing
		kidPath := filepath.Join(resolveKeypairDir(), "admin.kid")
		_ = os.WriteFile(kidPath, []byte(keyID+"\n"), 0o644) // best-effort: kid file is optional

		return nil
	},
}

var authRevokeCmd = &cobra.Command{
	Use:   "revoke <key-id>",
	Short: "Revoke an admin key",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newClient()
		if err != nil {
			return err
		}

		if err := c.RevokeAdminKey(cmd.Context(), args[0]); err != nil {
			return fmt.Errorf("revoke admin key: %w", err)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Admin key revoked: %s\n", args[0])
		return nil
	},
}

var authListCmd = &cobra.Command{
	Use:   "list",
	Short: "List active admin keys",
	RunE: func(cmd *cobra.Command, _ []string) error {
		c, err := newClient()
		if err != nil {
			return err
		}

		result, err := c.ListAdminKeys(cmd.Context())
		if err != nil {
			return fmt.Errorf("list admin keys: %w", err)
		}

		items, _ := result["items"].([]any)
		if len(items) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No active admin keys.")
			return nil
		}

		fmt.Fprintf(cmd.OutOrStdout(), "%-20s %-20s %-12s %s\n", "KEY ID", "NAME", "ALGORITHM", "CREATED")
		for _, item := range items {
			m, _ := item.(map[string]any)
			fmt.Fprintf(cmd.OutOrStdout(), "%-20s %-20s %-12s %s\n",
				m["key_id"], m["name"], m["algorithm"], m["created_at"])
		}
		return nil
	},
}

// resolveKeypairDir returns the directory for storing admin keypair files.
func resolveKeypairDir() string {
	if resolvedCtx != nil && resolvedStore != nil {
		return filepath.Join(resolvedStore.Dir(), resolvedCtx.Name)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".sukko")
}
