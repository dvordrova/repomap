package main

import (
	"example.com/typed-cobra/internal/cli"
	"example.com/typed-cobra/internal/lookalike"
	"github.com/spf13/cobra"
)

var directMainRoot = &cobra.Command{Use: "direct-main"}

func main() {
	_ = directMainRoot.Execute()
	func() {
		nested := &cobra.Command{Use: "nested-func"}
		_ = nested.Execute()
	}()
	lookalike.Start()
	cli.MustStart()
	cli.MustStartSharedRoot()
}
