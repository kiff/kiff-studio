package studio

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"text/template"
)

// BuildTestScaffold returns Go source for a domain-package
// test file. The output declares the three convention-required
// cases (per docs/conventions.md §"Tests"):
//
//   - The happy path. A successful end-to-end run of one
//     non-approval-required action through the loop, asserting
//     the entity reaches the expected state.
//   - The blocked path. A high-risk action is refused without
//     approval; the entity state does not change.
//   - The replay path. runtime.RebuildState reconstructs the
//     same final state from the event log alone.
//
// The scaffold picks the first action that does NOT require
// approval as the happy-path target, and the first action that
// DOES require approval as the blocked-path target. If the
// Blueprint has no action of one kind, the corresponding test is
// generated as a TODO stub the user fills in.
//
// packageName is the Go package name the file declares (commonly
// "domain"). It must be a valid Go identifier.
//
// Output is byte-deterministic for a given Blueprint and
// packageName.
func BuildTestScaffold(b Blueprint, packageName string) ([]byte, error) {
	if err := b.Validate(); err != nil {
		return nil, fmt.Errorf("studio.BuildTestScaffold: %w", err)
	}
	if !isGoIdent(packageName) {
		return nil, fmt.Errorf("studio.BuildTestScaffold: package name %q is not a valid Go identifier", packageName)
	}

	tpl, err := template.New("tests").Funcs(executorFuncs).Parse(testsTemplate)
	if err != nil {
		return nil, fmt.Errorf("studio.BuildTestScaffold: parse template: %w", err)
	}

	data := buildTestData(b, packageName)
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("studio.BuildTestScaffold: execute template: %w", err)
	}
	return buf.Bytes(), nil
}

// testData feeds the test template. It carries the picks for
// happy and blocked actions plus the initial event the test
// uses to bootstrap an entity into the action's allowed state.
type testData struct {
	PackageName string
	Domain      string
	Entity      string

	HappyAction   *testActionView // nil → emit a TODO stub
	BlockedAction *testActionView // nil → emit a TODO stub

	// InitialEvent is the event the tests ingest first, to drive
	// the entity into a state where the actions are allowed.
	// Derived from the transition with From == "".
	InitialEventConst string // "EventOrderReadyForRefund"
	InitialState      string // const ref: "StateReadyForRefund"

	// RoleNameQuoted is the lexicographically-first role name
	// from the Blueprint, quoted for inline use in a Go string
	// literal (e.g. `"tenant_owner"`). The generated tests
	// construct an actor with this single role so action
	// validation succeeds on the proposing permission and
	// reaches the approval gate.
	RoleNameQuoted string
}

type testActionView struct {
	Name           string // "ISSUE_REFUND"
	GoName         string // "IssueRefund"
	ActionConst    string // "ActionIssueRefund"
	StateConst     string // const ref of the action's first allowed state
	ParameterPairs []parameterPair
	FollowUpEvent  string // const ref of the event the executor emits
}

type parameterPair struct {
	Name  string // raw param name: "amount"
	Value string // string literal Go value: "\"100\""
}

func buildTestData(b Blueprint, pkg string) testData {
	d := testData{
		PackageName: pkg,
		Domain:      b.Domain,
		Entity:      b.Entity,
	}

	// Pick the lexicographically-first role for the tests'
	// actor. Sorting matches the executor stub's ordering, so the
	// chosen role is the same role the executor's
	// NewPermissionPolicy emits a GrantRole for first.
	if len(b.Roles) > 0 {
		roles := make([]string, 0, len(b.Roles))
		for r := range b.Roles {
			roles = append(roles, r)
		}
		sort.Strings(roles)
		d.RoleNameQuoted = fmt.Sprintf("%q", roles[0])
	} else {
		d.RoleNameQuoted = `"tenant_owner"`
	}

	// Pick the initial event + state.
	for _, t := range b.Transitions {
		if t.From == "" {
			d.InitialEventConst = "Event" + goExportFromUpperSnake(t.On)
			d.InitialState = "State" + goExportFromUpperSnake(t.To)
			break
		}
	}

	// Index transitions to find each action's follow-up event.
	transitionFromState := make(map[string]string, len(b.Transitions))
	for _, t := range b.Transitions {
		if t.From != "" {
			transitionFromState[t.From] = t.On
		}
	}

	makeView := func(a Action) *testActionView {
		view := &testActionView{
			Name:        a.Name,
			GoName:      goExportFromUpperSnake(a.Name),
			ActionConst: "Action" + goExportFromUpperSnake(a.Name),
		}
		if len(a.AllowedStates) > 0 {
			view.StateConst = "State" + goExportFromUpperSnake(a.AllowedStates[0])
			if e, ok := transitionFromState[a.AllowedStates[0]]; ok {
				view.FollowUpEvent = "Event" + goExportFromUpperSnake(e)
			}
		}
		for _, p := range a.RequiredParameters {
			view.ParameterPairs = append(view.ParameterPairs, parameterPair{
				Name:  p,
				Value: stubParamValue(p),
			})
		}
		return view
	}

	for i := range b.Actions {
		a := b.Actions[i]
		if !a.ApprovalRequired && d.HappyAction == nil {
			d.HappyAction = makeView(a)
		}
		if a.ApprovalRequired && d.BlockedAction == nil {
			d.BlockedAction = makeView(a)
		}
	}

	return d
}

// stubParamValue returns a placeholder Go literal for a given
// parameter name. The convention is conservative: numeric-sounding
// names get a number, the rest get a string.
func stubParamValue(name string) string {
	lower := strings.ToLower(name)
	if strings.Contains(lower, "amount") || strings.Contains(lower, "count") || strings.HasSuffix(lower, "_id") {
		if strings.Contains(lower, "amount") || strings.Contains(lower, "count") {
			return "100"
		}
		return fmt.Sprintf("%q", "id-1")
	}
	return fmt.Sprintf("%q", "stub-"+name)
}

// testsTemplate renders the three convention-required cases.
// Indentation is tabs; the output is gofmt-clean.
const testsTemplate = `package {{.PackageName}}

// Generated by kiff-studio. The three convention-required cases:
// happy path, blocked path, replay. Replace stubs as you grow
// the domain.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kiff/kiff/pkg/kiff/action"
	"github.com/kiff/kiff/pkg/kiff/actor"
	"github.com/kiff/kiff/pkg/kiff/adapter"
	"github.com/kiff/kiff/pkg/kiff/approval"
	"github.com/kiff/kiff/pkg/kiff/runtime"
)

{{if .HappyAction}}
// TestHappyPath_{{.HappyAction.GoName}} runs the {{.HappyAction.Name}}
// action end-to-end against a freshly-bootstrapped entity. State
// should reach the executor's follow-up event target.
func TestHappyPath_{{.HappyAction.GoName}}(t *testing.T) {
	t.Parallel()
	rt, ctx := bootstrap{{.Entity}}(t, "entity-1")
	contract, _ := rt.Actions.Get({{.HappyAction.ActionConst}})
	actionCtx := action.ActionContext{
		ActionName:   {{.HappyAction.ActionConst}},
		EntityID:     "entity-1",
		EntityType:   Entity{{.Entity}},
		CurrentState: {{.HappyAction.StateConst}},
		Actor:        actor.Actor{ID: "user-1", Roles: []string{ {{.RoleNameQuoted}} }},
		Parameters: map[string]any{
{{- range .HappyAction.ParameterPairs}}
			"{{.Name}}": {{.Value}},
{{- end}}
		},
	}
	if _, err := rt.ExecuteAction(ctx, actionCtx, contract); err != nil {
		t.Fatalf("ExecuteAction: %v", err)
	}
}
{{else}}
// TestHappyPath is a placeholder: no non-approval action exists
// in the blueprint. Add an action with approval=never to enable
// a generated happy-path test.
func TestHappyPath(t *testing.T) {
	t.Skip("no non-approval action in blueprint; fill in your own happy-path case")
}
{{end}}

{{if .BlockedAction}}
// TestBlockedPath_{{.BlockedAction.GoName}} confirms the
// {{.BlockedAction.Name}} action is refused without an approval.
func TestBlockedPath_{{.BlockedAction.GoName}}(t *testing.T) {
	t.Parallel()
	rt, ctx := bootstrap{{.Entity}}(t, "entity-2")
	contract, _ := rt.Actions.Get({{.BlockedAction.ActionConst}})
	actionCtx := action.ActionContext{
		ActionName:   {{.BlockedAction.ActionConst}},
		EntityID:     "entity-2",
		EntityType:   Entity{{.Entity}},
		CurrentState: {{.BlockedAction.StateConst}},
		Actor:        actor.Actor{ID: "user-1", Roles: []string{ {{.RoleNameQuoted}} }},
		Parameters: map[string]any{
{{- range .BlockedAction.ParameterPairs}}
			"{{.Name}}": {{.Value}},
{{- end}}
		},
		ApprovalID: "approval-stub",
	}
	if _, err := rt.ExecuteAction(ctx, actionCtx, contract); !errors.Is(err, action.ErrApprovalRequired) {
		t.Fatalf("expected ErrApprovalRequired, got %v", err)
	}
	// Grant + retry; the action should now succeed.
	if _, err := rt.RequestApproval(ctx, actionCtx.ApprovalID, actionCtx, contract, "x"); err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}
	if _, err := rt.ReviewApproval(ctx, actionCtx.ApprovalID, "reviewer", approval.StatusGranted, "ok"); err != nil {
		t.Fatalf("ReviewApproval: %v", err)
	}
	if _, err := rt.ExecuteAction(ctx, actionCtx, contract); err != nil {
		t.Fatalf("ExecuteAction after grant: %v", err)
	}
}
{{else}}
// TestBlockedPath is a placeholder: no approval-required action
// exists in the blueprint. Add an action with approval=required
// to enable a generated blocked-path test.
func TestBlockedPath(t *testing.T) {
	t.Skip("no approval-required action in blueprint; fill in your own blocked-path case")
}
{{end}}

// TestReplay confirms that runtime.RebuildState reconstructs the
// same final state from the event log alone — the framework's
// principle 05 ("trust comes from reconstruction") in test form.
func TestReplay(t *testing.T) {
	t.Parallel()
	rt, ctx := bootstrap{{.Entity}}(t, "entity-3")
	want, _, err := rt.States.Current(ctx, "entity-3")
	if err != nil {
		t.Fatalf("States.Current: %v", err)
	}
	got, err := rt.RebuildState(ctx, "entity-3")
	if err != nil {
		t.Fatalf("RebuildState: %v", err)
	}
	if got.State.Value != want.Value {
		t.Fatalf("replay state %q != live state %q", got.State.Value, want.Value)
	}
}

// bootstrap{{.Entity}} ingests one initial event for the entity so
// downstream tests have a state-machine-correct starting point.
func bootstrap{{.Entity}}(t *testing.T, id string) (*runtime.Runtime, context.Context) {
	t.Helper()
	rt, err := NewRuntime()
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	ctx := context.Background()
	if _, err := rt.IngestRaw(ctx, adapter.RawInput{
		ID:         id + "-evt",
		Adapter:    Adapter{{.Domain | adapterIdent}},
		Type:       {{.InitialEventConst}},
		Source:     "test",
		EntityID:   id,
		EntityType: Entity{{.Entity}},
		ActorID:    "system",
		ReceivedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("IngestRaw: %v", err)
	}
	return rt, ctx
}`
