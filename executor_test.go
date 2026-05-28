package studio

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// TestBuildExecutorStub_OutputParsesAsGo asserts the executor
// stub is syntactically valid Go. The conformance test against
// the framework's agentic-ops template (in conformance_test.go)
// asserts the deeper shape match; this test is the cheap
// "the file would compile" smoke check.
func TestBuildExecutorStub_OutputParsesAsGo(t *testing.T) {
	t.Parallel()
	got, err := BuildExecutorStub(canonicalRefundBlueprint(), "domain")
	if err != nil {
		t.Fatalf("BuildExecutorStub: %v", err)
	}
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, "domain.go", got, parser.AllErrors); err != nil {
		t.Fatalf("output is not valid Go:\n%s\nerror: %v", string(got), err)
	}
}

// TestBuildExecutorStub_OutputContainsCanonicalShapes asserts
// the generated source carries the convention-required pieces:
// the Contracts() aggregator, one factory per action, the seven
// contract fields in order, and the executor function with a
// FollowUpEvents emission.
func TestBuildExecutorStub_OutputContainsCanonicalShapes(t *testing.T) {
	t.Parallel()
	got, err := BuildExecutorStub(canonicalRefundBlueprint(), "domain")
	if err != nil {
		t.Fatalf("BuildExecutorStub: %v", err)
	}
	out := string(got)
	requirements := []string{
		"package domain",
		"func Contracts() []action.ActionContract {",
		"issueRefundContract()",
		"func issueRefundContract() action.ActionContract {",
		"Name:                ActionIssueRefund,",
		"AllowedStates:       []string{ StateReadyForRefund },",
		`RequiredParameters:  []string{ "amount", "reason" },`,
		"RequiredPermissions: []permission.Permission{ PermRefundFlowIssueRefund, PermRefundFlowIssueRefundApprove },",
		"Risk:                action.RiskHigh,",
		"ApprovalRequirement: action.ApprovalRequired,",
		"Executor: func(_ context.Context, ctx action.ActionContext)",
		"FollowUpEvents: []event.Event{",
		"Type:       EventOrderRefunded,",
		"EntityType: EntityOrder,",
		`Source:     "refund-flow/executor",`,
		// Wiring functions added when the executor stub became
		// the full domain file (NewRuntime + friends).
		"func NewRuntime() (*runtime.Runtime, error)",
		"func NewDefinition() (kiffdomain.Definition, error)",
		"func NewPermissionPolicy() *permission.SimplePolicy",
		"func NewInputAdapter() (adapter.Adapter, error)",
		`policy.GrantRole("tenant_owner", PermRefundFlowIssueRefund)`,
		`policy.GrantRole("tenant_owner", PermRefundFlowIssueRefundApprove)`,
		"AdapterRefundFlow",
		// Definition wiring includes the events and transitions
		// from the constants block.
		`Event(EventOrderReadyForRefund)`,
		`Transition(EventOrderReadyForRefund, "", StateReadyForRefund)`,
		`Allow(StateReadyForRefund, ActionIssueRefund)`,
	}
	for _, want := range requirements {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q\n--- output ---\n%s", want, out)
		}
	}
}

func TestBuildExecutorStub_RejectsInvalidPackageName(t *testing.T) {
	t.Parallel()
	_, err := BuildExecutorStub(canonicalRefundBlueprint(), "9bad")
	if err == nil || !strings.Contains(err.Error(), "valid Go identifier") {
		t.Fatalf("expected invalid-identifier error, got %v", err)
	}
}
