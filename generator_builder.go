package studio

import "strings"

// generator_builder.go — the deterministic generator. Turns a
// BuilderInput into a Blueprint without any model call. Carries
// forward the logic from apps/web/internal/onboarding/builder.go
// (the embedded Studio v0 from #181), extended to the
// multi-action / multi-role shape.
//
// The generator is byte-deterministic: identical input always
// produces an identical Blueprint. It does not call into any
// runtime, fetch any URL, or read the filesystem.

// defaultExecutorKey is what generateFromBuilder sets on each
// emitted Action when the BuilderInput doesn't otherwise carry
// an executor. Cloud's wrapper layer overrides this before
// calling BuildYAML when saving to a Cloud tenant; Framework-only
// adopters set their own.
//
// "cloud.proxy" is the historical default (carried forward from
// the embedded Studio v0). It maps to Cloud's generic
// pre-execution proposal gate executor — the customer's own app
// receives the gate's "allowed" / "approval_required" / "blocked"
// / "invalid" decision and runs its own side-effect function.
const defaultExecutorKey = "cloud.proxy"

// tenantOwnerRole is the canonical solo-signup role name from
// RFC 011 M1.2a. When BuilderInput.Roles is empty, the
// generator emits one tenant_owner role bound to every action's
// proposing permission and its .approve counterpart.
const tenantOwnerRole = "tenant_owner"

// generateFromBuilder is the deterministic generator's entry
// point. The caller (GenerateBlueprint) has already called
// BuilderInput.validate(); generateFromBuilder may assume the
// input is shape-correct.
//
// Output rules:
//
//   - Events are derived from entity + state, mirroring the
//     embedded Studio v0 behavior: "Order" + "READY" ->
//     "ORDER_READY". One event per state.
//   - Transitions: for each action, if Transitions is non-empty
//     it lands verbatim. Otherwise the generator falls back to
//     linear inference (state[i-1] → state[i]) anchored on the
//     action's first AllowedState — preserving the legacy
//     single-action behavior so existing tenants don't see a
//     shape change.
//   - Permissions: each action emits a paired
//     <workflow>.<action_lower> + <workflow>.<action_lower>.approve
//     pair. (RFC 011 M1.2a — emit the .approve permission even
//     when ApprovalRequired is false; the runtime simply doesn't
//     check it. Keeps the YAML shape canonical.)
//   - Roles: when BuilderInput.Roles is empty, one tenant_owner
//     role holds every action's proposing + .approve permissions
//     (the canonical solo-signup shape). When non-empty,
//     BuilderInput.Roles lands verbatim.
//   - Risk: derived from ActionInput.Risk if set, else from
//     ApprovalRequired (true → high, false → low). Matches
//     RFC 011 M1.2a's risk-derivation rule.
//   - Executor: defaultExecutorKey ("cloud.proxy") for every
//     action. Caller can override the Blueprint's
//     Action[i].Executor before rendering.
func generateFromBuilder(b BuilderInput) Blueprint {
	entityUpper := strings.ToUpper(camelToSnake(b.EntityType))
	events := make([]string, 0, len(b.States))
	for _, s := range b.States {
		events = append(events, entityUpper+"_"+s)
	}

	bp := Blueprint{
		Domain:      b.WorkflowName,
		Entity:      b.EntityType,
		Events:      events,
		States:      append([]string(nil), b.States...),
		Transitions: deriveTransitions(b, events),
		Actions:     buildActions(b),
		Roles:       buildRoles(b),
	}
	return bp
}

// deriveTransitions returns the Blueprint's transition list. If
// any ActionInput carries explicit Transitions, all are honored
// (and merged across actions). Otherwise the generator falls
// back to linear inference: events[0] from "" to states[0],
// then events[i] from states[i-1] to states[i]. This preserves
// the legacy single-action behavior when no explicit
// transitions are set.
func deriveTransitions(b BuilderInput, events []string) []Transition {
	var explicit []Transition
	for _, a := range b.Actions {
		for _, t := range a.Transitions {
			explicit = append(explicit, Transition{On: t.On, From: t.From, To: t.To})
		}
	}
	if len(explicit) > 0 {
		return explicit
	}

	// Linear fallback. Mirrors the embedded Studio v0 behavior.
	out := make([]Transition, 0, len(b.States))
	out = append(out, Transition{On: events[0], From: "", To: b.States[0]})
	for i := 1; i < len(b.States); i++ {
		out = append(out, Transition{On: events[i], From: b.States[i-1], To: b.States[i]})
	}
	return out
}

// buildActions translates each ActionInput into a Blueprint
// Action. Permissions are paired (proposing + .approve);
// risk is derived; executor key is the package default.
func buildActions(b BuilderInput) []Action {
	out := make([]Action, 0, len(b.Actions))
	for _, a := range b.Actions {
		actionLower := strings.ToLower(a.Name)
		propose := b.WorkflowName + "." + actionLower
		approve := propose + ".approve"

		risk := a.Risk
		if risk == "" {
			if a.ApprovalRequired {
				risk = "high"
			} else {
				risk = "low"
			}
		}

		out = append(out, Action{
			Name:                a.Name,
			AllowedStates:       append([]string(nil), a.AllowedStates...),
			RequiredParameters:  append([]string(nil), a.RequiredParameters...),
			RequiredPermissions: []string{propose, approve},
			Risk:                risk,
			ApprovalRequired:    a.ApprovalRequired,
			Executor:            defaultExecutorKey,
		})
	}
	return out
}

// buildRoles produces the Blueprint's Roles map. When the input
// lists roles, they land verbatim. When empty, the canonical
// solo-signup shape is emitted: one tenant_owner role bound to
// every action's proposing + .approve permissions.
func buildRoles(b BuilderInput) map[string][]string {
	if len(b.Roles) > 0 {
		out := make(map[string][]string, len(b.Roles))
		for _, r := range b.Roles {
			out[r.Name] = append([]string(nil), r.Permissions...)
		}
		return out
	}

	// Default: tenant_owner holds every action's pair.
	perms := make([]string, 0, 2*len(b.Actions))
	for _, a := range b.Actions {
		actionLower := strings.ToLower(a.Name)
		propose := b.WorkflowName + "." + actionLower
		perms = append(perms, propose, propose+".approve")
	}
	return map[string][]string{tenantOwnerRole: perms}
}
