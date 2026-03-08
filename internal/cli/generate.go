package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/vector76/backlog_manager/internal/client"
)

func newStartGenerateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start-generate <feature-id>",
		Short: "Transition a feature from ready_to_generate to generating",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client.New()
			if c.Token == "" {
				return fmt.Errorf("BM_TOKEN is required (set env var or .env file)")
			}
			result, err := c.StartGenerate(args[0])
			if err != nil {
				return err
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		},
	}
}

func newRegisterBeadsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "register-beads <feature-id> <bead-id> [<bead-id>...]",
		Short: "Store bead IDs on a feature and transition generating -> beads_created",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client.New()
			if c.Token == "" {
				return fmt.Errorf("BM_TOKEN is required (set env var or .env file)")
			}
			featureID := args[0]
			beadIDs := args[1:]
			result, err := c.RegisterBeads(featureID, beadIDs)
			if err != nil {
				return err
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		},
	}
}

func newRegisterArtifactCmd() *cobra.Command {
	var artifactType, filePath string
	cmd := &cobra.Command{
		Use:   "register-artifact <feature-id>",
		Short: "Store a plan or beads artifact file for a feature",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client.New()
			if c.Token == "" {
				return fmt.Errorf("BM_TOKEN is required (set env var or .env file)")
			}
			if artifactType != "plan" && artifactType != "beads" {
				return fmt.Errorf("--type must be 'plan' or 'beads'")
			}
			if filePath == "" {
				return fmt.Errorf("--file is required")
			}
			content, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("read file: %w", err)
			}
			if err := c.RegisterArtifact(args[0], artifactType, string(content)); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "{}")
			return nil
		},
	}
	cmd.Flags().StringVar(&artifactType, "type", "", "Artifact type: 'plan' or 'beads' (required)")
	cmd.Flags().StringVar(&filePath, "file", "", "Path to the artifact file (required)")
	return cmd
}

func newCompleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "complete <feature-id>",
		Short: "Transition a feature from beads_created to done",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client.New()
			if c.Token == "" {
				return fmt.Errorf("BM_TOKEN is required (set env var or .env file)")
			}
			result, err := c.CompleteFeature(args[0])
			if err != nil {
				return err
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		},
	}
}
