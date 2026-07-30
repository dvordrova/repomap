package cli

import "github.com/spf13/cobra"

func newPutCommand() *cobra.Command {
	return &cobra.Command{Use: "put <key> <value>", Run: runPut}
}

func newSnapshotCommand() *cobra.Command {
	command := &cobra.Command{Use: "snapshot <subcommand>"}
	command.AddCommand(newSnapshotSaveCommand())
	return command
}

func newSnapshotSaveCommand() *cobra.Command {
	return &cobra.Command{Use: "save <filename>", Run: runSnapshotSave}
}

func newUserCommand() *cobra.Command {
	command := &cobra.Command{Use: "user <subcommand>"}
	command.AddCommand(newUserAddCommand())
	return command
}

func newUserAddCommand() *cobra.Command {
	return &cobra.Command{Use: "add <name>", RunE: runUserAdd}
}

func newRoleCommand() *cobra.Command {
	command := &cobra.Command{Use: "role <subcommand>"}
	command.AddCommand(newRoleGrantPermissionCommand())
	return command
}

func newRoleGrantPermissionCommand() *cobra.Command {
	return &cobra.Command{Use: "grant-permission <role>", Run: runRoleGrantPermission}
}

func runPut(*cobra.Command, []string) {}

func runSnapshotSave(*cobra.Command, []string) {}

func runUserAdd(*cobra.Command, []string) error {
	return nil
}

func runRoleGrantPermission(*cobra.Command, []string) {}

func init() {
	rootCmd.AddCommand(newDeleteCommand())
}

func newDeleteCommand() *cobra.Command {
	return &cobra.Command{Use: "del <key>", RunE: runDelete}
}

func runDelete(*cobra.Command, []string) error {
	return nil
}
