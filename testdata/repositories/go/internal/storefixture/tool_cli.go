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
// asked once. Each case's branch runs its subcommand's own code.
func RunSubcommand(args []string) string {
	var cmd string
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}
	switch cmd {
	case "serve":
		return runServe(args)
	case "check", "verify":
		return runCheck(args)
	default:
		if cmd == "help" || cmd == "-h" {
			return "usage"
		}
		return "unknown command " + cmd
	}
}

// runServe is serve's own code, as each litestream command's Run is: only
// the serve case runs it, so the flags its own flag set declares are
// serve's options. -verbose is declared by check too, as litestream's
// commands each declare -json: two options, one of each subcommand.
func runServe(args []string) string {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	verbose := fs.Bool("verbose", false, "log each request")
	addCommon(fs)
	_ = fs.Parse(args)
	if *verbose {
		return "serve verbosely " + strings.Join(fs.Args(), " ")
	}
	return "serve " + strings.Join(fs.Args(), " ")
}

// runCheck is check's own code: its -verbose is check's option.
func runCheck(args []string) string {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	verbose := fs.Bool("verbose", false, "log each check")
	addCommon(fs)
	_ = fs.Parse(args)
	if *verbose {
		return "check verbosely"
	}
	return "check"
}

// addCommon declares -quiet on the flag set it is handed, as a tool shares
// its subcommands' common flags: serve's and check's own code each hand it
// their flag set, and only their cases run that code, so -quiet is an
// option of each and no flag of the tool's own.
func addCommon(fs *flag.FlagSet) {
	fs.Bool("quiet", false, "print nothing")
}

// IsDefaultLevel compares a level with one word: a lone comparison is no
// dispatch and records nothing.
func IsDefaultLevel(level string) bool {
	return level == "default"
}

// LooksLikeFlag tells an argument placed after the positional ones by its
// first character, as litestream's replicate does: "-" is a mark a flag
// starts with, never a word of its own.
func LooksLikeFlag(arg string) bool {
	return strings.HasPrefix(arg, "-")
}
