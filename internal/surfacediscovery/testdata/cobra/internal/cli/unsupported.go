package cli

import "github.com/spf13/cobra"

func init() {
	rootCmd.AddCommand(
		newDynamicCommand(),
		newConditionalCommand(),
		newMutatedGroupCommand(),
	)
	if dynamicUse() != "" {
		callback := func() {}
		callback()
	}
	rootCmd.AddCommand(newAfterConditionalLiteralCommand())
	mutateSeparatePutInstance()
}

// registerDeadCommand is intentionally never called. Merely importing this
// package must not make its AddCommand edge part of the rooted command tree.
func registerDeadCommand() {
	rootCmd.AddCommand(newDeadCommand())
	rootCmd.Use = dynamicUse()
	rootCmd.Run = dynamicHandler()
}

func newDeadCommand() *cobra.Command {
	return &cobra.Command{Use: "dead", Run: runDead}
}

// newDynamicCommand starts with exact fields, then replaces them with values
// the bounded static inventory cannot resolve. The later writes must invalidate
// the earlier exact facts.
func newDynamicCommand() *cobra.Command {
	command := &cobra.Command{Use: "before-dynamic", Run: runDynamic}
	command.Use = dynamicUse()
	command.Run = dynamicHandler()
	return command
}

func newConditionalCommand() *cobra.Command {
	command := &cobra.Command{Use: "before-conditional", Run: runConditional}
	if dynamicUse() != "" {
		command.Use = "after-conditional"
		command.Run = runConditionalAfter
	}
	return command
}

func newMutatedGroupCommand() *cobra.Command {
	child := newExternallyMutatedCommand()
	child.Run = dynamicHandler()
	command := &cobra.Command{Use: "mutated-group"}
	command.AddCommand(child)
	return command
}

func newExternallyMutatedCommand() *cobra.Command {
	return &cobra.Command{Use: "mutated-child", Run: runInitiallyExact}
}

func newAfterConditionalLiteralCommand() *cobra.Command {
	return &cobra.Command{Use: "after-conditional-literal", Run: runAfterConditionalLiteral}
}

// mutateSeparatePutInstance models the etcdctl shape that mutates one
// constructor invocation. That instance-specific write must not overwrite the
// allocation-site metadata used by the separately rooted put command.
func mutateSeparatePutInstance() {
	clone := newPutCommand()
	clone.Run = dynamicHandler()
	deleteClone := newDeleteCommand()
	deleteClone.RunE = dynamicHandlerE()
	orphan := &cobra.Command{Use: "orphan"}
	orphan.AddCommand(clone, deleteClone)
}

func dynamicUse() string {
	return "dynamic-at-runtime"
}

func dynamicHandler() func(*cobra.Command, []string) {
	return runDynamic
}

func dynamicHandlerE() func(*cobra.Command, []string) error {
	return runDelete
}

func runDead(*cobra.Command, []string) {}

func runDynamic(*cobra.Command, []string) {}

func runConditional(*cobra.Command, []string) {}

func runConditionalAfter(*cobra.Command, []string) {}

func runInitiallyExact(*cobra.Command, []string) {}

func runAfterConditionalLiteral(*cobra.Command, []string) {}
