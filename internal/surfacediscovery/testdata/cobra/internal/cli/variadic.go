package cli

import "github.com/spf13/cobra"

func init() {
	addCommands(
		rootCmd,
		newVariadicAlphaCommand(),
		newVariadicBetaCommand(),
	)
	localChildren := []*cobra.Command{
		newVariadicGammaCommand(),
		newVariadicDeltaCommand(),
	}
	addCommands(rootCmd, localChildren...)
	rootCmd.AddCommand(dynamicVariadicCommands()...)
}

func addCommands(parent *cobra.Command, children ...*cobra.Command) {
	forwardCommands(parent, children...)
}

func forwardCommands(parent *cobra.Command, children ...*cobra.Command) {
	parent.AddCommand(children...)
}

func newVariadicAlphaCommand() *cobra.Command {
	return &cobra.Command{Use: "variadic-alpha", Run: runVariadicAlpha}
}

func newVariadicBetaCommand() *cobra.Command {
	return &cobra.Command{Use: "variadic-beta", RunE: runVariadicBeta}
}

func newVariadicGammaCommand() *cobra.Command {
	return &cobra.Command{Use: "variadic-gamma", Run: runVariadicGamma}
}

func newVariadicDeltaCommand() *cobra.Command {
	return &cobra.Command{Use: "variadic-delta", RunE: runVariadicDelta}
}

func dynamicVariadicCommands() []*cobra.Command {
	return nil
}

func runVariadicAlpha(*cobra.Command, []string) {}

func runVariadicBeta(*cobra.Command, []string) error {
	return nil
}

func runVariadicGamma(*cobra.Command, []string) {}

func runVariadicDelta(*cobra.Command, []string) error {
	return nil
}
