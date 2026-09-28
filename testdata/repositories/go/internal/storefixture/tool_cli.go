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
