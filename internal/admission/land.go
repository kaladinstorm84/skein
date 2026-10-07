package admission

import (
	"skein/internal/canonical"
	"skein/internal/skeinerr"
)

func LandCAS(currentWeaveID string, proposal map[string]any, policyOK, gateOK, integrationOK bool) (map[string]any, error) {
	target, _ := canonical.AsString(proposal["target_weave_id"])
	if target != currentWeaveID {
		return nil, skeinerr.New(skeinerr.Stale, "target weave advanced; rebuild Proposal and WeaveProposal")
	}
	if !policyOK {
		return nil, skeinerr.New(skeinerr.Policy, "pinned validation policy unsatisfied")
	}
	if !integrationOK {
		return nil, skeinerr.New(skeinerr.Policy, "integration witness did not pass for composed state")
	}
	if !gateOK {
		return nil, skeinerr.New(skeinerr.Unauthenticated, "gate did not verify over WeaveProposal hash")
	}
	return map[string]any{
		"action":          "land",
		"parent_weave_id": currentWeaveID,
	}, nil
}
