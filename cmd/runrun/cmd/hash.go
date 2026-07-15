// Package cmd implements the RunRun CLI commands: server, hash-password, and version.
package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/sgaunet/runrun/internal/auth"
	"github.com/spf13/cobra"
)

// errInvalidCost is returned when the requested BCrypt cost is out of range.
var errInvalidCost = errors.New("invalid cost (must be 4-31)")

var (
	cost int
)

// hashPasswordCmd represents the hash-password command.
var hashPasswordCmd = &cobra.Command{
	Use:   "hash-password <password>",
	Short: "Generate BCrypt hash for a password",
	Long: `Generate a BCrypt hash for a password that can be used in the
configuration file for user authentication.

The cost parameter controls the computational cost of hashing. Higher
values provide more security but take longer to compute. Default is 10.

Valid cost range: 4-31`,
	Args: cobra.ExactArgs(1),
	RunE: hashPassword,
	Example: `  # Hash a password with default cost (10)
  runrun hash-password mypassword

  # Hash a password with custom cost
  runrun hash-password --cost=12 mypassword

  # Use in configuration
  runrun hash-password mysecret
  # Copy the output to config.yaml under auth.users.<username>.password`,
}

func init() {
	rootCmd.AddCommand(hashPasswordCmd)

	// Flags for hash-password command
	hashPasswordCmd.Flags().IntVarP(&cost, "cost", "c", auth.DefaultCost, "BCrypt cost factor (4-31)")
}

func hashPassword(_ *cobra.Command, args []string) error {
	password := args[0]

	// Validate password strength
	if err := auth.ValidatePasswordStrength(password); err != nil {
		return fmt.Errorf("password validation failed: %w", err)
	}

	// Validate cost parameter
	if cost < 4 || cost > 31 {
		return fmt.Errorf("cost %d: %w", cost, errInvalidCost)
	}

	// Hash password
	hash, err := auth.HashPasswordWithCost(password, cost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Output the hash
	if _, err := fmt.Fprintln(os.Stdout, hash); err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}

	return nil
}
