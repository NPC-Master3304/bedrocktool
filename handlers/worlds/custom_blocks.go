package worlds

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/df-mc/dragonfly/server/world"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
)

// traitLookup maps block trait identifiers to the set of values they expand to.
// Block traits are a compact way for a server to describe common property sets
// (such as facing directions) without listing every permutation explicitly.
var traitLookup = map[string][]any{
	"minecraft:facing_direction": {
		"north", "east", "south", "west", "down", "up",
	},
	"minecraft:cardinal_direction": {
		"north", "east", "south", "west",
	},
	"minecraft:vertical_half": {
		"top", "bottom",
	},
	"minecraft:block_face": {
		"north", "east", "south", "west", "down", "up",
	},
}

// splitNamespace splits an identifier such as "minecraft:stone" into its
// namespace ("minecraft") and name ("stone"). Identifiers without a namespace
// return the same value for both name components.
func splitNamespace(identifier string) (ns, name string) {
	parts := strings.Split(identifier, ":")
	return parts[0], parts[len(parts)-1]
}

// addCustomBlocks registers every non-vanilla block sent by the server (in the
// StartGame packet) with the block registry so that proxied chunks referencing
// those runtime IDs can be resolved.
//
// This was previously provided by dragonfly's world.AddCustomBlocks, which was
// dropped when the fork merged the 1.26.20 upstream changes, so it lives here
// now and uses the public block registry API.
func addCustomBlocks(reg world.BlockRegistry, entries []protocol.BlockEntry) error {
	for _, entry := range entries {
		ns, _ := splitNamespace(entry.Name)
		if ns == "minecraft" {
			continue
		}

		var propertyNames []string
		var propertyValues []any

		if props, ok := entry.Properties["properties"].([]any); ok {
			for _, v := range props {
				v := v.(map[string]any)
				propertyNames = append(propertyNames, v["name"].(string))
				propertyValues = append(propertyValues, v["enum"])
			}
		}

		if traits, ok := entry.Properties["traits"].([]any); ok {
			for _, trait := range traits {
				trait := trait.(map[string]any)
				enabledStates := trait["enabled_states"].(map[string]any)
				for k, enabled := range enabledStates {
					if !strings.ContainsRune(k, ':') {
						k = "minecraft:" + k
					}
					if enabled.(uint8) == 0 {
						continue
					}
					v, ok := traitLookup[k]
					if !ok {
						return fmt.Errorf("unresolved trait %s", k)
					}
					propertyNames = append(propertyNames, k)
					propertyValues = append(propertyValues, v)
				}
			}
		}

		for _, values := range cartesianProduct(propertyValues) {
			m := make(map[string]any, len(values))
			for i, value := range values {
				m[propertyNames[i]] = value
			}
			reg.RegisterBlockState(world.BlockState{
				Name:       entry.Name,
				Properties: m,
			})
		}
	}

	return nil
}

// cartesianProduct returns every combination picking one value from each of the
// slices in sets. Each element of sets is expected to be a slice; the result is
// a list of value tuples in the same order as sets. An empty input yields a
// single empty tuple, matching the behaviour of a block with no properties.
func cartesianProduct(sets []any) [][]any {
	result := [][]any{{}}
	for _, set := range sets {
		values := toAnySlice(set)
		next := make([][]any, 0, len(result)*max(len(values), 1))
		for _, combo := range result {
			for _, value := range values {
				row := make([]any, len(combo), len(combo)+1)
				copy(row, combo)
				next = append(next, append(row, value))
			}
		}
		result = next
	}
	return result
}

// toAnySlice normalises a value that holds a slice (of any concrete element
// type, as produced by NBT decoding) into a []any.
func toAnySlice(v any) []any {
	if s, ok := v.([]any); ok {
		return s
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice {
		return nil
	}
	out := make([]any, rv.Len())
	for i := range out {
		out[i] = rv.Index(i).Interface()
	}
	return out
}
