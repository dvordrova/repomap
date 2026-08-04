// Package surfacebridge adapts grounded surface records to current flow inputs.
package surfacebridge

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/flowexplain"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

func FlowSeed(trigger surfacediscovery.TriggerRecord) (flowexplain.FlowSeed, error) {
	if trigger.ID == "" || trigger.Kind != "http_route" {
		return flowexplain.FlowSeed{}, fmt.Errorf("surface bridge: a grounded http route trigger is required")
	}
	if trigger.RegistrationSite.Path == "" || trigger.RegistrationSite.Line <= 0 {
		return flowexplain.FlowSeed{}, fmt.Errorf("surface bridge: trigger %q has no exact registration site", trigger.ID)
	}
	identity := strings.TrimSpace(trigger.Identity.Method + " " + trigger.Identity.Path.Text)
	if identity == "" {
		identity = "dynamic HTTP registration"
	}
	files := []string{trigger.RegistrationSite.Path}
	if trigger.ProcessEntrypoint.Location.Path != "" {
		files = append(files, trigger.ProcessEntrypoint.Location.Path)
	}
	for _, wrapper := range trigger.WrapperChain {
		if wrapper.Symbol.Location.Path != "" {
			files = append(files, wrapper.Symbol.Location.Path)
		}
	}
	sort.Strings(files)
	files = unique(files)
	evidence := []string{
		fmt.Sprintf(
			"configured terminal %s reached at %s:%d with %s certainty and %s resolution",
			trigger.FinalSeed,
			trigger.RegistrationSite.Path,
			trigger.RegistrationSite.Line,
			trigger.Certainty,
			trigger.Resolution,
		),
	}
	if trigger.Handler.Text != "" {
		evidence = append(evidence, "registered handler: "+trigger.Handler.Text)
	}
	if trigger.Dispatcher.Text != "" {
		evidence = append(evidence, "dispatcher: "+trigger.Dispatcher.Text)
	}
	return flowexplain.FlowSeed{
		ID:               trigger.ID,
		Name:             identity,
		Trigger:          "registered HTTP route " + identity,
		LikelyEntrypoint: trigger.ProcessEntrypoint.Location.Path,
		ValidSeedFiles:   files,
		UnverifiedSeeds:  []string{},
		Evidence:         evidence,
	}, nil
}

func unique(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}
