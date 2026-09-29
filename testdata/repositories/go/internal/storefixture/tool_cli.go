package storefixture

import (
	"encoding/json"
	"flag"
	"os"
	"strings"
)

// ServerConfig is the file an operator writes for the server: the keys its
// tags name are that file's settings, since LoadServerConfig decodes the
// file into it. The response structures' tags name keys of data the
// program sends, not settings.
type ServerConfig struct {
	Listen  string `json:"listen"`
	DataDir string `json:"data_dir"`
}

// LoadServerConfig reads the configuration file.
func LoadServerConfig(path string) (ServerConfig, error) {
	var config ServerConfig
	raw, err := os.ReadFile(path)
	if err != nil {
		return config, err
	}
	err = json.Unmarshal(raw, &config)
	return config, err
}

// ToolCommand reads a tool's command line: the subcommand is compared with
// the program's first argument, and each subcommand's flags are declared on
// the flag set NewFlagSet makes for it, two objects in one function. The
// same comparison of a level's own name with a word is no option.
func ToolCommand(args []string, level string) (int, bool) {
	serve := flag.NewFlagSet("serve", flag.ContinueOnError)
	port := serve.Int("port", 8080, "port to listen on")
	check := flag.NewFlagSet("check", flag.ContinueOnError)
	strict := check.Bool("strict", false, "fail on warnings")
	if len(os.Args) > 1 && strings.EqualFold(os.Args[1], "check") {
		_ = check.Parse(args)
		return 0, *strict
	}
	_ = serve.Parse(args)
	return *port, strings.EqualFold(level, "default")
}

// toolCommand is one subcommand of a tool, as litestream writes each of its
// commands: Usage prints the command's flags when they are wrong.
type toolCommand struct {
	name string
}

func (c *toolCommand) Usage() {}

// Run hands the command's usage printer to its flag set by storing it in the
// flag set's field. The store names that field, flag.FlagSet.Usage, declared
// func(): what the stored callable becomes is asked of the field, not of the
// flag set.
func (c *toolCommand) Run(args []string) error {
	fs := flag.NewFlagSet(c.name, flag.ContinueOnError)
	fs.Usage = c.Usage
	return fs.Parse(args)
}

// RunTool runs the tool's one command.
func RunTool(args []string) error {
	return (&toolCommand{name: "tool"}).Run(args)
}

// RunSubcommand runs the subcommand its first argument names, as
// litestream's Main.Run does: the parallel assignment takes that argument,
// the switch compares it with each subcommand's name ("check" and "verify"
// are one case), and the default branch compares it with the help words,
// one case of the same value. The whole dispatch is one comparison of cmd,
// asked once.
func RunSubcommand(args []string) string {
	var cmd string
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}
	switch cmd {
	case "serve":
		return "serve " + strings.Join(args, " ")
	case "check", "verify":
		return "check"
	default:
		if cmd == "help" || cmd == "-h" {
			return "usage"
		}
		return "unknown command " + cmd
	}
}

// IsDefaultLevel compares a level with one word: a lone comparison is no
// dispatch and records nothing.
func IsDefaultLevel(level string) bool {
	return level == "default"
}
