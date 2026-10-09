package atlas

import "fmt"

func validateZoneTree(zones []Zone) error {
	byID := map[string]Zone{}
	for _, zone := range zones {
		byID[zone.ID] = zone
	}
	for _, zone := range zones {
		seen := map[string]bool{zone.ID: true}
		for at := zone.ParentID; at != ""; at = byID[at].ParentID {
			if _, known := byID[at]; !known {
				return fmt.Errorf("zone %q has unknown parent %q", zone.ID, at)
			}
			if seen[at] {
				return fmt.Errorf("cyclic zone tree")
			}
			seen[at] = true
		}
	}
	var count func(string) int
	count = func(id string) int {
		n := len(byID[id].BoxIDs)
		for _, child := range zones {
			if child.ParentID == id {
				n += count(child.ID)
			}
		}
		return n
	}
	for _, zone := range zones {
		if count(zone.ID) < 2 {
			return fmt.Errorf("zone %q has fewer than two descendant boxes", zone.ID)
		}
		if zone.ParentID != "" && count(zone.ID) >= count(zone.ParentID) {
			return fmt.Errorf("zone %q repeats its parent scope", zone.ID)
		}
	}
	return nil
}
