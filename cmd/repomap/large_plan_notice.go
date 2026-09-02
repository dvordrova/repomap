package main

import "fmt"

// largePlanRequests is when a batch stops being something a reader waits
// through and starts being something they would want to know the size of
// first. A 26,218-object repository planned hundreds of categorization
// requests and said nothing until they had all been made; on a cache miss
// that is real money spent before the first line of output.
//
// It is a reporting threshold only. Nothing is sampled, capped or removed:
// every planned request is still made.
const largePlanRequests = 24

// largePlanNotice reports the size of a batch before it runs, so a reader can
// stop a run that is about to cost more than they meant to spend.
func largePlanNotice(output *runOutput, stage, target string) func(int) {
	if output == nil {
		return nil
	}
	return func(requests int) {
		if requests < largePlanRequests {
			return
		}
		output.Warn(
			"Large provider request plan",
			"stage: "+stage,
			"target: "+target,
			fmt.Sprintf("provider requests: %d", requests),
			"cached requests cost nothing; uncached ones are live calls",
		)
	}
}
