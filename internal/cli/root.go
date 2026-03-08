package cli

import "github.com/spf13/cobra"

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "bm",
		Short: "Backlog manager — server and client",
	}
	root.AddCommand(newServeCmd())
	root.AddCommand(newStatusCmd())
	root.AddCommand(newFeaturesCmd())
	root.AddCommand(newShowCmd())
	root.AddCommand(newPollCmd())
	root.AddCommand(newFetchCmd())
	root.AddCommand(newSubmitCmd())
	root.AddCommand(newStartGenerateCmd())
	root.AddCommand(newRegisterBeadsCmd())
	root.AddCommand(newRegisterArtifactCmd())
	root.AddCommand(newCompleteCmd())
	return root
}
