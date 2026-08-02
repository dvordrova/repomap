package cli

import "github.com/spf13/cobra"

var initialized = registerInitializedCommand()

func registerInitializedCommand() bool {
	rootCmd.AddCommand(newInitializedCommand())
	return true
}

func newInitializedCommand() *cobra.Command {
	return &cobra.Command{Use: "initialized", Run: runInitialized}
}

func runInitialized(*cobra.Command, []string) {}
