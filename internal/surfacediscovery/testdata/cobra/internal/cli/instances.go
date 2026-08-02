package cli

import "github.com/spf13/cobra"

var (
	executedSharedRoot   = newSharedRootCommand()
	unexecutedSharedRoot = newSharedRootCommand()
)

func init() {
	executedSharedRoot.AddCommand(newOnlyACommand())
	unexecutedSharedRoot.AddCommand(newOnlyBCommand())
}

func MustStartSharedRoot() {
	if err := executedSharedRoot.Execute(); err != nil {
		panic(err)
	}
}

func newSharedRootCommand() *cobra.Command {
	return &cobra.Command{Use: "shared-root"}
}

func newOnlyACommand() *cobra.Command {
	return &cobra.Command{Use: "only-a", Run: runOnlyA}
}

func newOnlyBCommand() *cobra.Command {
	return &cobra.Command{Use: "only-b", Run: runOnlyB}
}

func runOnlyA(*cobra.Command, []string) {}

func runOnlyB(*cobra.Command, []string) {}
