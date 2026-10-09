package main

import rand "math/rand/v2"

// runtimeSampler is stored behind an interface. Native SSA must retain its
// ordinary methods and the method set of its standard-library field, even
// when a newer compiler adds a generic method to rand.Rand.
type runtimeSampler struct {
	random *rand.Rand
}

func (sampler *runtimeSampler) Sample() int {
	return sampler.random.IntN(10)
}

// Reset is deliberately outside the launch tree; its original body and helper
// remain native evidence rather than being removed to avoid reflection work.
func (sampler *runtimeSampler) Reset() {
	resetRuntimeSampler(sampler)
}

func resetRuntimeSampler(sampler *runtimeSampler) {
	sampler.random = rand.New(rand.NewPCG(1, 2))
}

var visibleRuntimeSampler any = &runtimeSampler{
	random: rand.New(rand.NewPCG(1, 2)),
}
