package lifecycle

type Module struct{}

func (Module) Provision() {}

func Start() {
	Module{}.Provision()
}
