package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/vector76/backlog_manager/internal/client"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Query own project info",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client.New()
			if c.Token == "" {
				return fmt.Errorf("BM_TOKEN is required (set env var or .env file)")
			}
			result, err := c.GetOwnProject()
			if err != nil {
				return err
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		},
	}
}
