package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{Use: "fixture"}

var (
	ambiguousA = &cobra.Command{Use: "ambiguous-a"}
	ambiguousB = &cobra.Command{Use: "ambiguous-b"}
)

func init() {
	localRoot := rootCmd
	localRoot.AddCommand(
		newGetCommand(),
		newPutCommand(),
		newLeaseCommand(),
		newEndpointCommand(),
		newSnapshotCommand(),
		newUserCommand(),
		newRoleCommand(),
	)

	var ambiguous *cobra.Command
	if errors.Is(nil, nil) {
		ambiguous = ambiguousA
	} else {
		ambiguous = ambiguousB
	}
	ambiguous.AddCommand(newGhostCommand())
}

func Start() error {
	return rootCmd.Execute()
}

func MustStart() {
	if err := Start(); err != nil {
		panic(err)
	}
}
