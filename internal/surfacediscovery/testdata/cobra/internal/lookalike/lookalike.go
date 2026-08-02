package lookalike

type Command struct {
	Use string
	Run func(*Command, []string)
}

func (command *Command) AddCommand(...*Command) {}

func (command *Command) Execute() error {
	return nil
}

var root = &Command{Use: "fake"}

func Start() {
	root.AddCommand(&Command{Use: "also-fake"})
	_ = root.Execute()
}
