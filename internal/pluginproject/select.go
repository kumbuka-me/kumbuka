package pluginproject

import (
	"fmt"
	"slices"
	"strings"
)

// Select returns requested plugins and their transitive manifest dependencies.
func Select(packages []Resolved, ids []string) ([]Resolved, error) {
	catalog := make(map[string]Resolved, len(packages))
	for _, item := range packages {
		catalog[item.Manifest.ID] = item
	}

	state := make(map[string]int)
	selected := make(map[string]Resolved)
	var visit func(string) error
	visit = func(id string) error {
		switch state[id] {
		case 2:
			return nil
		case 1:
			return fmt.Errorf("plugin dependency cycle at %s", id)
		}
		item, found := catalog[id]
		if !found {
			return fmt.Errorf("required plugin %s is not declared in %s", id, DefaultFile)
		}
		state[id] = 1
		for _, dependency := range item.Manifest.Requires {
			if err := visit(dependency); err != nil {
				return fmt.Errorf("plugin %s: %w", id, err)
			}
		}
		state[id] = 2
		selected[id] = item
		return nil
	}

	for _, id := range ids {
		if err := visit(id); err != nil {
			return nil, err
		}
	}

	result := make([]Resolved, 0, len(selected))
	for _, item := range selected {
		result = append(result, item)
	}
	slices.SortFunc(result, func(left, right Resolved) int {
		return strings.Compare(left.Manifest.ID, right.Manifest.ID)
	})
	return result, nil
}
