package app

import "sort"

func operationsForPhase(sync map[string]any, phase string) []map[string]any {
	items, _ := sync["operations"].([]any)
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if entry["phase"] != phase {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func operationPaths(sync map[string]any, phase string) []string {
	ops := operationsForPhase(sync, phase)
	out := make([]string, 0, len(ops))
	for _, op := range ops {
		path, _ := op["path"].(string)
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

func findOperation(sync map[string]any, phase, path string) map[string]any {
	for _, op := range operationsForPhase(sync, phase) {
		if op["path"] == path {
			return op
		}
	}
	return nil
}
