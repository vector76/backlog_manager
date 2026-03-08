package cli

import "github.com/spf13/cobra"

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "bm",
		Short: "Backlog manager — server and client",
	}
	root.AddCommand(newServeCmd())
	root.AddCommand(newStatusCmd())
	return root
}
