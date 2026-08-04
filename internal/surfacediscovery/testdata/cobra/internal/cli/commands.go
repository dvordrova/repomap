package cli

import "github.com/spf13/cobra"

func newGetCommand() *cobra.Command {
	return &cobra.Command{
		Use:  "get <key>",
		RunE: runGet,
	}
}

func newLeaseCommand() *cobra.Command {
	command := &cobra.Command{Use: "lease <subcommand>"}
	command.AddCommand(newLeaseGrantCommand())
	return command
}

func newLeaseGrantCommand() *cobra.Command {
	return &cobra.Command{
		Use: "grant <ttl>",
		Run: runLeaseGrant,
	}
}

func newEndpointCommand() *cobra.Command {
	command := &cobra.Command{Use: "endpoint <subcommand>"}
	command.AddCommand(&cobra.Command{
		Use: "health",
		Run: func(*cobra.Command, []string) {
		},
	})
	return command
}

func newGhostCommand() *cobra.Command {
	return &cobra.Command{Use: "ghost", Run: runGhost}
}

func runGet(*cobra.Command, []string) error {
	return nil
}

func runLeaseGrant(*cobra.Command, []string) {}

func runGhost(*cobra.Command, []string) {}
