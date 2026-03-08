package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/vector76/backlog_manager/internal/client"
)

func newPollCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "poll",
		Short: "Block until work is available, then print action JSON to stdout",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client.New()
			if c.Token == "" {
				return fmt.Errorf("BM_TOKEN is required (set env var or .env file)")
			}
			// Loop until work is available (server returns 204 on timeout; client retries).
			for {
				result, err := c.Poll()
				if err != nil {
					return err
				}
				if result != nil {
					enc := json.NewEncoder(cmd.OutOrStdout())
					enc.SetIndent("", "  ")
					return enc.Encode(result)
				}
				// 204 timeout: server found no work; retry immediately.
			}
		},
	}
}

func newFetchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "fetch <feature-id>",
		Short: "Get pending work for a feature, print to stdout",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client.New()
			if c.Token == "" {
				return fmt.Errorf("BM_TOKEN is required (set env var or .env file)")
			}
			result, err := c.FetchPending(args[0])
			if err != nil {
				return err
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		},
	}
}

func newSubmitCmd() *cobra.Command {
	var descFile, questionsFile string
	cmd := &cobra.Command{
		Use:   "submit <feature-id>",
		Short: "Submit dialog results from local files",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client.New()
			if c.Token == "" {
				return fmt.Errorf("BM_TOKEN is required (set env var or .env file)")
			}
			if descFile == "" {
				return fmt.Errorf("--description is required")
			}
			descContent, err := os.ReadFile(descFile)
			if err != nil {
				return fmt.Errorf("read description file: %w", err)
			}
			var questionsContent string
			if questionsFile != "" {
				data, err := os.ReadFile(questionsFile)
				if err != nil {
					return fmt.Errorf("read questions file: %w", err)
				}
				questionsContent = string(data)
			}
			result, err := c.SubmitDialog(args[0], string(descContent), questionsContent)
			if err != nil {
				return err
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		},
	}
	cmd.Flags().StringVar(&descFile, "description", "", "Path to updated description markdown file (required)")
	cmd.Flags().StringVar(&questionsFile, "questions", "", "Path to questions markdown file (optional)")
	return cmd
}
