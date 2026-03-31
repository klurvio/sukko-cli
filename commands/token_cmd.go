package commands

import (
	"errors"
	"fmt"
	"time"

	clitoken "github.com/klurvio/sukko-cli/token"
	"github.com/spf13/cobra"
)

var (
	tokenSub       string
	tokenTenant    string
	tokenRoles     []string
	tokenGroups    []string
	tokenScopes    []string
	tokenTTL       time.Duration
	tokenKeyFile   string
	tokenAlgorithm string
)

func init() {
	tokenCmd.AddCommand(tokenGenerateCmd, tokenValidateCmd)

	tokenGenerateCmd.Flags().StringVar(&tokenSub, "sub", "", "Subject (user ID)")
	tokenGenerateCmd.Flags().StringVar(&tokenTenant, "tenant", "", "Tenant ID")
	tokenGenerateCmd.Flags().StringSliceVar(&tokenRoles, "roles", nil, "Roles (repeatable)")
	tokenGenerateCmd.Flags().StringSliceVar(&tokenGroups, "groups", nil, "Groups (repeatable)")
	tokenGenerateCmd.Flags().StringSliceVar(&tokenScopes, "scopes", nil, "Scopes (repeatable)")
	tokenGenerateCmd.Flags().DurationVar(&tokenTTL, "ttl", time.Hour, "Token time-to-live (e.g., 1h, 30m, 24h)")
	tokenGenerateCmd.Flags().StringVar(&tokenKeyFile, "key-file", "", "Path to PEM private key")
	tokenGenerateCmd.Flags().StringVar(&tokenAlgorithm, "algorithm", "", "Signing algorithm (ES256, RS256, EdDSA)")

	tokenValidateCmd.Flags().StringVar(&tokenKeyFile, "key-file", "", "Path to PEM public key (for signature verification)")

	rootCmd.AddCommand(tokenCmd)
}

var tokenCmd = &cobra.Command{
	Use:   "token",
	Short: "JWT token helpers",
}

var tokenGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate a signed JWT token",
	RunE: func(cmd *cobra.Command, _ []string) error {
		algorithm := tokenAlgorithm
		keyFile := tokenKeyFile

		// Auto-discover stored key if no --algorithm and no --key-file
		if algorithm == "" && keyFile == "" {
			tenant := resolveTenant(tokenTenant)
			if tenant != "" {
				if path, err := loadLatestPrivateKey(tenant); err == nil && path != "" {
					algorithm = "ES256"
					keyFile = path
				}
			}
		}

		if algorithm == "" || keyFile == "" {
			return errors.New("specify --algorithm (ES256, RS256, EdDSA) and --key-file, or run 'sukko key create --generate' first. HS256 is not supported by the Sukko gateway")
		}

		validAlgorithms := map[string]bool{
			"ES256": true, "RS256": true, "EdDSA": true,
		}
		if !validAlgorithms[algorithm] {
			return fmt.Errorf("invalid algorithm %q: must be one of ES256, RS256, EdDSA. HS256 is not supported by the Sukko gateway", algorithm)
		}

		tenant := resolveTenant(tokenTenant)

		cfg := clitoken.GenerateConfig{
			Subject:   tokenSub,
			TenantID:  tenant,
			Roles:     tokenRoles,
			Groups:    tokenGroups,
			Scopes:    tokenScopes,
			TTL:       tokenTTL,
			KeyFile:   keyFile,
			Algorithm: algorithm,
		}

		tokenStr, err := clitoken.Generate(cfg)
		if err != nil {
			return fmt.Errorf("generate token: %w", err)
		}

		fmt.Fprintln(cmd.OutOrStdout(), tokenStr)
		return nil
	},
}

var tokenValidateCmd = &cobra.Command{
	Use:   "validate <token>",
	Short: "Decode and validate a JWT token",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		tokenStr := args[0]

		var result *clitoken.DecodedToken
		var err error
		verified := false
		if tokenKeyFile != "" {
			result, err = clitoken.ValidateWithKeyFile(tokenStr, tokenKeyFile)
			verified = true
		} else {
			result, err = clitoken.Decode(tokenStr)
		}
		if err != nil {
			return fmt.Errorf("validate token: %w", err)
		}

		if output == "json" {
			return printJSON(result)
		}

		// Human-readable output
		fmt.Fprintln(cmd.OutOrStdout(), "Header:")
		for k, v := range result.Header {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s: %v\n", k, v)
		}

		fmt.Fprintln(cmd.OutOrStdout(), "\nClaims:")
		for k, v := range result.Claims {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s: %v\n", k, v)
		}

		if result.Valid {
			fmt.Fprintf(cmd.OutOrStdout(), "\nStatus: %svalid%s", colorGreen, colorReset)
			if verified {
				fmt.Fprint(cmd.OutOrStdout(), " (signature verified)")
			} else {
				fmt.Fprint(cmd.OutOrStdout(), " (signature not verified — use --key-file to verify)")
			}
			fmt.Fprintln(cmd.OutOrStdout())
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "\nStatus: %sinvalid%s (%s)\n", colorRed, colorReset, result.Error)
		}

		return nil
	},
}
