package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/vector76/backlog_manager/internal/client"
)

func newFeaturesCmd() *cobra.Command {
	var statusFilter string
	cmd := &cobra.Command{
		Use:   "features",
		Short: "List features for the project",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client.New()
			if c.Token == "" {
				return fmt.Errorf("BM_TOKEN is required (set env var or .env file)")
			}
			result, err := c.ListFeatures(statusFilter)
			if err != nil {
				return err
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		},
	}
	cmd.Flags().StringVar(&statusFilter, "status", "", "Filter by comma-separated statuses (e.g. draft,awaiting_client)")
	return cmd
}

func newShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <feature-id>",
		Short: "Show feature details including description and dialog history",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client.New()
			if c.Token == "" {
				return fmt.Errorf("BM_TOKEN is required (set env var or .env file)")
			}
			result, err := c.GetFeatureDetail(args[0])
			if err != nil {
				return err
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		},
	}
}
