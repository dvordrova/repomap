package cli

import "github.com/spf13/cobra"

var reassignedRoot = &cobra.Command{Use: "reassigned-before"}

func init() {
	reassignedRoot = &cobra.Command{Use: "reassigned-after"}
	_ = reassignedRoot.Execute()
}

func executeParameter(command *cobra.Command) error {
	return command.Execute()
}
