package studio

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// TestBuildTestScaffold_OutputParsesAsGo asserts the generated
// test file is syntactically valid Go.
func TestBuildTestScaffold_OutputParsesAsGo(t *testing.T) {
	t.Parallel()
	got, err := BuildTestScaffold(canonicalRefundBlueprint(), "domain")
	if err != nil {
		t.Fatalf("BuildTestScaffold: %v", err)
	}
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, "domain_test.go", got, parser.AllErrors); err != nil {
		t.Fatalf("output is not valid Go:\n%s\nerror: %v", string(got), err)
	}
}

// TestBuildTestScaffold_ContainsThreeConventionRequiredCases
// asserts the output declares the three test functions the
// framework's conventions require.
func TestBuildTestScaffold_ContainsThreeConventionRequiredCases(t *testing.T) {
	t.Parallel()
	got, err := BuildTestScaffold(canonicalRefundBlueprint(), "domain")
	if err != nil {
		t.Fatalf("BuildTestScaffold: %v", err)
	}
	out := string(got)
	requirements := []string{
		// Happy-path test: there's no non-approval action in the
		// canonical blueprint, so the scaffold emits a TODO stub.
		"func TestHappyPath(t *testing.T)",
		`t.Skip("no non-approval action in blueprint`,
		// Blocked-path test: the canonical action is approval-required.
		"func TestBlockedPath_IssueRefund(t *testing.T)",
		"action.ErrApprovalRequired",
		"approval.StatusGranted",
		// Replay test: the framework's RebuildState in the convention shape.
		"func TestReplay(t *testing.T)",
		"rt.RebuildState(ctx, ",
		// bootstrap helper.
		"func bootstrapOrder(t *testing.T, id string)",
		"AdapterRefundFlow",
		"EventOrderReadyForRefund",
	}
	for _, want := range requirements {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q\n--- output ---\n%s", want, out)
		}
	}
}

// TestBuildTestScaffold_BothActionTypesPresent_HappyPathRendered
// covers the case where the blueprint carries both an
// approval-required and a non-approval action: both test
// functions render without TODO stubs.
func TestBuildTestScaffold_BothActionTypesPresent_HappyPathRendered(t *testing.T) {
	t.Parallel()
	bp := canonicalRefundBlueprint()
	// Add a low-risk action so the happy path has a target.
	bp.Actions = append(bp.Actions, Action{
		Name:                "MARK_READY",
		AllowedStates:       []string{"REFUNDED"},
		RequiredParameters:  []string{"reason"},
		RequiredPermissions: []string{"refund-flow.mark_ready", "refund-flow.mark_ready.approve"},
		Risk:                "low",
		ApprovalRequired:    false,
		Executor:            "cloud.proxy",
	})
	got, err := BuildTestScaffold(bp, "domain")
	if err != nil {
		t.Fatalf("BuildTestScaffold: %v", err)
	}
	out := string(got)
	if !strings.Contains(out, "func TestHappyPath_MarkReady(t *testing.T)") {
		t.Fatalf("expected happy-path test for MarkReady, got:\n%s", out)
	}
	if strings.Contains(out, `t.Skip("no non-approval action`) {
		t.Fatalf("expected no skip stub when a happy-path action exists")
	}
}
