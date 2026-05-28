package studio

import (
	"strings"
	"testing"
)

// TestBlueprint_Validate_HappyPath exercises the canonical
// refund-flow shape. A Blueprint with two states, one action,
// transitions wired, the canonical paired permissions, and the
// tenant_owner role should validate.
func TestBlueprint_Validate_HappyPath(t *testing.T) {
	t.Parallel()
	bp := canonicalRefundBlueprint()
	if err := bp.Validate(); err != nil {
		t.Fatalf("Validate: unexpected error %v", err)
	}
}

func TestBlueprint_Validate_RejectsBadDomainName(t *testing.T) {
	t.Parallel()
	bp := canonicalRefundBlueprint()
	bp.Domain = "BadName" // PascalCase is invalid
	err := bp.Validate()
	if err == nil || !strings.Contains(err.Error(), "domain") {
		t.Fatalf("expected domain validation error, got %v", err)
	}
}

func TestBlueprint_Validate_RejectsTooFewStates(t *testing.T) {
	t.Parallel()
	bp := canonicalRefundBlueprint()
	bp.States = []string{"ONLY_ONE"}
	err := bp.Validate()
	if err == nil || !strings.Contains(err.Error(), "two states") {
		t.Fatalf("expected too-few-states error, got %v", err)
	}
}

func TestBlueprint_Validate_RejectsActionWithUnknownAllowedState(t *testing.T) {
	t.Parallel()
	bp := canonicalRefundBlueprint()
	bp.Actions[0].AllowedStates = []string{"NOT_A_STATE"}
	err := bp.Validate()
	if err == nil || !strings.Contains(err.Error(), "allowed_state") {
		t.Fatalf("expected unknown-allowed-state error, got %v", err)
	}
}

func TestBlueprint_Validate_RejectsActionMissingExecutor(t *testing.T) {
	t.Parallel()
	bp := canonicalRefundBlueprint()
	bp.Actions[0].Executor = ""
	err := bp.Validate()
	if err == nil || !strings.Contains(err.Error(), "executor") {
		t.Fatalf("expected missing-executor error, got %v", err)
	}
}

func TestBlueprint_Validate_RejectsBadPermission(t *testing.T) {
	t.Parallel()
	bp := canonicalRefundBlueprint()
	bp.Roles["tenant_owner"] = []string{"BadPermission"}
	err := bp.Validate()
	if err == nil || !strings.Contains(err.Error(), "dotted.lowercase") {
		t.Fatalf("expected bad-permission error, got %v", err)
	}
}

// TestBlueprint_Validate_RejectsTransitionWithUnknownEvent ensures
// transitions reference declared events.
func TestBlueprint_Validate_RejectsTransitionWithUnknownEvent(t *testing.T) {
	t.Parallel()
	bp := canonicalRefundBlueprint()
	bp.Transitions[0].On = "UNDECLARED_EVENT"
	err := bp.Validate()
	if err == nil || !strings.Contains(err.Error(), "transition.on") {
		t.Fatalf("expected unknown-transition-event error, got %v", err)
	}
}

// canonicalRefundBlueprint is the test fixture used across the
// blueprint, generator, and yaml tests. It mirrors the framework's
// examples/refund/refund.go shape (modulo names) and the post-#203
// canonical permission shape.
func canonicalRefundBlueprint() Blueprint {
	return Blueprint{
		Domain: "refund-flow",
		Entity: "Order",
		Events: []string{"ORDER_READY_FOR_REFUND", "ORDER_REFUNDED"},
		States: []string{"READY_FOR_REFUND", "REFUNDED"},
		Transitions: []Transition{
			{On: "ORDER_READY_FOR_REFUND", From: "", To: "READY_FOR_REFUND"},
			{On: "ORDER_REFUNDED", From: "READY_FOR_REFUND", To: "REFUNDED"},
		},
		Actions: []Action{
			{
				Name:               "ISSUE_REFUND",
				AllowedStates:      []string{"READY_FOR_REFUND"},
				RequiredParameters: []string{"amount", "reason"},
				RequiredPermissions: []string{
					"refund-flow.issue_refund",
					"refund-flow.issue_refund.approve",
				},
				Risk:             "high",
				ApprovalRequired: true,
				Executor:         "cloud.proxy",
			},
		},
		Roles: map[string][]string{
			"tenant_owner": {
				"refund-flow.issue_refund",
				"refund-flow.issue_refund.approve",
			},
		},
	}
}
