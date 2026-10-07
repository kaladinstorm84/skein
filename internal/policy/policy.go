package policy

import (
	"skein/internal/canonical"
)

var ScaleRank = map[string]int{
	"trivial":  0,
	"standard": 1,
	"deep":     2,
}

func Evaluate(policy map[string]any, scale string, evidence map[string]any) []string {
	scales, _ := canonical.AsMap(policy["scales"])
	required, ok := canonical.AsMap(scales[scale])
	if !ok {
		return []string{"unknown_scale"}
	}
	var missing []string
	need := func(flag string, present bool) {
		if b, ok := canonical.AsBool(required[flag]); ok && b && !present {
			missing = append(missing, flag)
		}
	}
	need("patch", truthy(evidence["patch"]))
	need("test_witness", truthy(evidence["test_witness"]))
	need("integration_witness", truthy(evidence["integration_witness_pass"]))
	need("land_gate", truthy(evidence["land_gate"]))
	need("story", truthy(evidence["story"]))
	need("acceptance", truthy(evidence["acceptance"]))
	need("verification_witnesses", truthy(evidence["verification_witnesses"]))
	need("independent_review", truthy(evidence["independent_review"]))
	need("adversarial_witness", truthy(evidence["adversarial_witness"]))
	need("brief_or_requirement", truthy(evidence["brief_or_requirement"]))
	need("readiness_gate", truthy(evidence["readiness_gate"]))
	need("second_human_land_signer", truthy(evidence["second_human_land_signer"]))
	if v, ok := canonical.AsBool(evidence["witness_authenticated"]); ok && !v {
		missing = append(missing, "authenticated_witness")
	}
	if truthy(evidence["automated_scale_down"]) {
		missing = append(missing, "human_required_for_scale_down")
	}
	if truthy(evidence["gate_reuse_requested"]) {
		missing = append(missing, "gate_reuse_refused")
	}
	return missing
}

func DeriveThreadState(flags map[string]any) string {
	if truthy(flags["abandoned"]) {
		return "abandoned"
	}
	if truthy(flags["landed"]) {
		return "landed"
	}
	if truthy(flags["gated"]) {
		return "gated"
	}
	if truthy(flags["in_review"]) {
		return "in-review"
	}
	if truthy(flags["building"]) {
		return "building"
	}
	if truthy(flags["ready"]) {
		return "ready"
	}
	return "draft"
}

func EffectiveScale(assertions []map[string]any) string {
	current := "trivial"
	for _, item := range assertions {
		if !truthy(item["valid"]) {
			continue
		}
		scale, _ := canonical.AsString(item["scale"])
		rank, ok := ScaleRank[scale]
		if !ok {
			continue
		}
		cur := ScaleRank[current]
		issuer, _ := canonical.AsString(item["issuer_class"])
		if issuer == "" {
			issuer = "human"
		}
		if issuer != "human" {
			if rank > cur {
				current = scale
			}
			continue
		}
		if rank < cur && !truthy(item["authorized_down"]) {
			continue
		}
		current = scale
	}
	return current
}

func truthy(v any) bool {
	b, ok := canonical.AsBool(v)
	return ok && b
}
