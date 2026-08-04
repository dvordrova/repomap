package errgroup

type Group struct{}

func (group *Group) Go(callback func() error) {}
