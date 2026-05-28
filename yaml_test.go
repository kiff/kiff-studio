package studio

import (
	"context"
	"strings"
	"testing"
)

// canonicalRefundYAML is the byte-exact output BuildYAML must
// produce for canonicalRefundBlueprint. The framework's
// docs/conventions.md is the source of truth for naming and
// shape; this fixture captures one canonical instance so any
// drift in BuildYAML's output (whitespace, ordering, missing
// fields) is a test failure.
//
// The shape mirrors what apps/api/internal/domain/builder_yaml_test.go
// pins for the embedded Studio v0 — the cloud's contract test
// in MVP-3 (#212) replaces its duplicated YAML literal with a
// call to BuildYAML(canonicalRefundBlueprint()) once kiff-studio
// is published.
const canonicalRefundYAML = `domain: refund-flow
entity: Order

events:
  - ORDER_READY_FOR_REFUND
  - ORDER_REFUNDED

states:
  - READY_FOR_REFUND
  - REFUNDED

transitions:
  - on: ORDER_READY_FOR_REFUND
    from: ""
    to: READY_FOR_REFUND
  - on: ORDER_REFUNDED
    from: READY_FOR_REFUND
    to: REFUNDED

actions:
  - name: ISSUE_REFUND
    allowed_states: [READY_FOR_REFUND]
    required_parameters: [amount, reason]
    required_permissions: [refund-flow.issue_refund, refund-flow.issue_refund.approve]
    risk: high
    approval: required
    executor: cloud.proxy

permissions:
  roles:
    tenant_owner:
      - refund-flow.issue_refund
      - refund-flow.issue_refund.approve
`

func TestBuildYAML_MatchesCanonicalRefundShape(t *testing.T) {
	t.Parallel()
	got, err := BuildYAML(canonicalRefundBlueprint())
	if err != nil {
		t.Fatalf("BuildYAML: %v", err)
	}
	if string(got) != canonicalRefundYAML {
		t.Fatalf("YAML mismatch.\n--- got ---\n%s\n--- want ---\n%s", string(got), canonicalRefundYAML)
	}
}

// TestBuildYAML_BuilderInputRoundTrip exercises the full
// authoring path: BuilderInput → GenerateBlueprint → BuildYAML
// → canonical bytes. This is the path Cloud's onboarding form
// will take after MVP-2.
func TestBuildYAML_BuilderInputRoundTrip(t *testing.T) {
	t.Parallel()
	bp, err := GenerateBlueprint(context.Background(), canonicalRefundInput())
	if err != nil {
		t.Fatalf("GenerateBlueprint: %v", err)
	}
	got, err := BuildYAML(bp)
	if err != nil {
		t.Fatalf("BuildYAML: %v", err)
	}
	if string(got) != canonicalRefundYAML {
		t.Fatalf("YAML mismatch on round-trip.\n--- got ---\n%s\n--- want ---\n%s", string(got), canonicalRefundYAML)
	}
}

// TestBuildYAML_RejectsInvalidBlueprint asserts that BuildYAML
// surfaces validation errors from the Blueprint rather than
// emitting partially-rendered output.
func TestBuildYAML_RejectsInvalidBlueprint(t *testing.T) {
	t.Parallel()
	bp := canonicalRefundBlueprint()
	bp.Actions[0].Executor = ""
	_, err := BuildYAML(bp)
	if err == nil || !strings.Contains(err.Error(), "executor") {
		t.Fatalf("expected executor validation error, got %v", err)
	}
}
