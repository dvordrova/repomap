package dead

import "context"

// Plain and Contextual deliberately have different Start signatures and are
// never called. They exercise declaration-family classification without
// inventing a call-target or shared-signature proof.
type Plain struct{}

func (Plain) Start() {}

type Contextual struct{}

func (Contextual) Start(context.Context) {}
