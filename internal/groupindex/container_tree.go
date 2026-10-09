package groupindex

import "fmt"

// ContainerGroups returns original leaf groups in saved tree order. GroupIDs
// always records direct ownership; aggregation never becomes new membership.
func ContainerGroups(containers []Container, id string) []string {
	var result []string
	seen := map[string]bool{}
	var visit func(string)
	visit = func(at string) {
		if seen[at] {
			return
		}
		seen[at] = true
		for _, container := range containers {
			if container.ID == at {
				result = append(result, container.GroupIDs...)
				break
			}
		}
		for _, container := range containers {
			if container.ParentID == at {
				visit(container.ID)
			}
		}
	}
	visit(id)
	return result
}

func validateContainerTree(containers []Container) error {
	parents := map[string]string{}
	for _, c := range containers {
		parents[c.ID] = c.ParentID
	}
	for _, c := range containers {
		seen := map[string]bool{c.ID: true}
		for at := c.ParentID; at != ""; at = parents[at] {
			if _, known := parents[at]; !known {
				return fmt.Errorf("group index: container %q has unknown parent %q", c.ID, at)
			}
			if seen[at] {
				return fmt.Errorf("group index: cyclic container tree")
			}
			seen[at] = true
		}
		if len(ContainerGroups(containers, c.ID)) < 2 {
			return fmt.Errorf("group index: container %q has fewer than two descendant groups", c.ID)
		}
		if c.ParentID != "" && len(ContainerGroups(containers, c.ID)) >= len(ContainerGroups(containers, c.ParentID)) {
			return fmt.Errorf("group index: container %q repeats its parent scope", c.ID)
		}
	}
	return nil
}

func pruneEmptyContainers(containers []Container) []Container {
	result := []Container{}
	for _, c := range containers {
		if len(ContainerGroups(containers, c.ID)) > 0 {
			result = append(result, c)
		}
	}
	return result
}
