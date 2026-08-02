package cobra

type Command struct {
	Use  string
	Run  func(*Command, []string)
	RunE func(*Command, []string) error
}

func (command *Command) AddCommand(...*Command) {}

func (command *Command) Execute() error {
	return nil
}

func (command *Command) ExecuteContext(any) error {
	return nil
}
