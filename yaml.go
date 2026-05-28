package studio

import (
	"fmt"
	"sort"
	"strings"
)

// BuildYAML returns a kiff.yaml derived from the Blueprint. The
// output is byte-deterministic: identical Blueprint always
// produces identical bytes. The function does not parse YAML —
// it composes a known-good string and returns it.
//
// The output's shape matches what the framework's
// domain.ParseAndBuild accepts (see apps/api/internal/domain/spec.go
// in kiff-cloud). It also matches the canonical permission shape
// from RFC 011 M1.2a: every action declares both its proposing
// and its .approve permission, and the default tenant_owner role
// holds both for every action.
//
// BuildYAML calls Blueprint.Validate first; an invalid Blueprint
// is returned as the validation error.
func BuildYAML(b Blueprint) ([]byte, error) {
	if err := b.Validate(); err != nil {
		return nil, fmt.Errorf("studio.BuildYAML: %w", err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("domain: %s\n", b.Domain))
	sb.WriteString(fmt.Sprintf("entity: %s\n\n", b.Entity))

	sb.WriteString("events:\n")
	for _, e := range b.Events {
		sb.WriteString(fmt.Sprintf("  - %s\n", e))
	}
	sb.WriteString("\n")

	sb.WriteString("states:\n")
	for _, s := range b.States {
		sb.WriteString(fmt.Sprintf("  - %s\n", s))
	}
	sb.WriteString("\n")

	sb.WriteString("transitions:\n")
	for _, t := range b.Transitions {
		sb.WriteString(fmt.Sprintf("  - on: %s\n", t.On))
		if t.From == "" {
			sb.WriteString(`    from: ""` + "\n")
		} else {
			sb.WriteString(fmt.Sprintf("    from: %s\n", t.From))
		}
		sb.WriteString(fmt.Sprintf("    to: %s\n", t.To))
	}
	sb.WriteString("\n")

	sb.WriteString("actions:\n")
	for _, a := range b.Actions {
		sb.WriteString(fmt.Sprintf("  - name: %s\n", a.Name))
		sb.WriteString(fmt.Sprintf("    allowed_states: [%s]\n", strings.Join(a.AllowedStates, ", ")))
		if len(a.RequiredParameters) > 0 {
			sb.WriteString(fmt.Sprintf("    required_parameters: [%s]\n", strings.Join(a.RequiredParameters, ", ")))
		} else {
			sb.WriteString("    required_parameters: []\n")
		}
		sb.WriteString(fmt.Sprintf("    required_permissions: [%s]\n", strings.Join(a.RequiredPermissions, ", ")))
		sb.WriteString(fmt.Sprintf("    risk: %s\n", a.Risk))
		approval := "never"
		if a.ApprovalRequired {
			approval = "required"
		}
		sb.WriteString(fmt.Sprintf("    approval: %s\n", approval))
		sb.WriteString(fmt.Sprintf("    executor: %s\n", a.Executor))
	}
	sb.WriteString("\n")

	if len(b.Roles) > 0 {
		sb.WriteString("permissions:\n")
		sb.WriteString("  roles:\n")
		// Sort role names for deterministic output. Map iteration
		// order in Go is randomized; without sorting the YAML
		// would change byte-for-byte across calls with the same
		// Blueprint, breaking goldens and conformance tests.
		roleNames := sortedKeys(b.Roles)
		for _, role := range roleNames {
			sb.WriteString(fmt.Sprintf("    %s:\n", role))
			for _, perm := range b.Roles[role] {
				sb.WriteString(fmt.Sprintf("      - %s\n", perm))
			}
		}
	}

	return []byte(sb.String()), nil
}

// sortedKeys returns the map keys in stable lexicographic order.
// Used by BuildYAML so role iteration is byte-deterministic.
func sortedKeys(m map[string][]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
