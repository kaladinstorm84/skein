package graph

import (
	"skein/internal/canonical"
	"skein/internal/object"
)

func WhyChain(path string, lands []map[string]any) (map[string]any, error) {
	target, err := object.NormalizePath(path)
	if err != nil {
		return nil, err
	}
	for i := len(lands) - 1; i >= 0; i-- {
		land := lands[i]
		raw, _ := canonical.AsSlice(land["patch_paths"])
		for _, p := range raw {
			s, _ := canonical.AsString(p)
			if s == target {
				parents := []any{}
				if ps, ok := canonical.AsSlice(land["parents"]); ok {
					parents = ps
				}
				id, _ := canonical.AsString(land["land_id"])
				return map[string]any{
					"path":    target,
					"land_id": id,
					"parents": parents,
				}, nil
			}
		}
	}
	return nil, nil
}
