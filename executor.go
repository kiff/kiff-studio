package studio

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"text/template"
)

// BuildExecutorStub returns Go source for a domain-package
// actions file. The output declares one factory function per
// action (`<actionName>Contract()`) that returns an
// action.ActionContract with a working executor function. The
// executor body is a stub: it succeeds, emits one follow-up
// event named after the post-execution transition's `On` event,
// and includes a TODO comment marking where the user wires real
// side-effect logic.
//
// The output also includes a top-level `Contracts() []action.ActionContract`
// aggregator that returns every action's contract — the same
// shape the framework's `examples/refund/refund.go` and the
// agentic-ops template produce. A consumer's `NewDefinition`
// iterates over `Contracts()` to register them.
//
// packageName is the Go package name the file declares (commonly
// "domain"). It must be a valid Go identifier.
//
// Output is byte-deterministic for a given Blueprint and
// packageName.
func BuildExecutorStub(b Blueprint, packageName string) ([]byte, error) {
	if err := b.Validate(); err != nil {
		return nil, fmt.Errorf("studio.BuildExecutorStub: %w", err)
	}
	if !isGoIdent(packageName) {
		return nil, fmt.Errorf("studio.BuildExecutorStub: package name %q is not a valid Go identifier", packageName)
	}

	tpl, err := template.New("executor").Funcs(executorFuncs).Parse(executorTemplate)
	if err != nil {
		// Programmer error: a hard-coded template should not fail
		// to parse. Surface as an error rather than panicking so
		// tests catch it.
		return nil, fmt.Errorf("studio.BuildExecutorStub: parse template: %w", err)
	}

	data := executorData{
		PackageName:  packageName,
		Domain:       b.Domain,
		Entity:       b.Entity,
		AdapterIdent: goExportName(b.Domain),
		Events:       buildEventConstRefs(b),
		Transitions:  buildTransitionViews(b),
		Allows:       buildAllowViews(b),
		Actions:      buildActionViews(b),
		Roles:        buildRoleViews(b),
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("studio.BuildExecutorStub: execute template: %w", err)
	}
	return buf.Bytes(), nil
}

// executorData is what the executor template renders against.
type executorData struct {
	PackageName  string
	Domain       string
	Entity       string
	AdapterIdent string   // PascalCase Go identifier for the adapter const
	Events       []string // const refs: "EventOrderRefunded"
	Transitions  []executorTransitionView
	Allows       []executorAllowView
	Actions      []executorActionView
	Roles        []executorRoleView
}

// executorTransitionView is one transition rule with const refs
// resolved for the kiffdomain.New(...).Transition(...) call.
type executorTransitionView struct {
	OnConst   string // "EventOrderReadyForRefund"
	FromConst string // "" or "StateReadyForRefund" (already quoted/literal)
	ToConst   string // "StateReadyForRefund"
}

// executorAllowView is one Allow(state, action) registration
// derived from the state machine's allowed-actions map.
type executorAllowView struct {
	StateConst  string
	ActionConst string
}

// executorRoleView is one role's permission grants. Each entry's
// permission is the const ref the constants block exports.
type executorRoleView struct {
	NameQuoted string   // `"tenant_owner"`
	PermConsts []string // ["PermRefundFlowIssueRefund", ...]
}

// executorActionView is the per-action shape the template needs.
// Pre-computed so the template stays declarative and the
// derivation logic stays in Go.
type executorActionView struct {
	Name             string   // "ISSUE_REFUND"
	GoName           string   // "IssueRefund" (used in factory name + const ref)
	ActionConst      string   // "ActionIssueRefund"
	AllowedStates    []string // const refs: "StateReadyForRefund"
	Parameters       []string // raw param names: "amount", "reason"
	PermissionConsts []string // const refs: "PermRefundFlowIssueRefund"
	Risk             string   // "action.RiskHigh"
	Approval         string   // "action.ApprovalRequired"
	FollowUpEvent    string   // const ref of the event the executor emits, or "" if none derivable
}

// buildActionViews pre-computes the per-action template view.
// Const refs are derived from the same naming functions
// constants.go uses, so the executor stub references constants
// the constants file declares.
func buildActionViews(b Blueprint) []executorActionView {
	// Index transitions by from-state to find the post-execution
	// event each action emits. The convention is: the action runs
	// in AllowedStates[0]; the transition that exits that state
	// (From == AllowedStates[0]) names the follow-up event.
	transitionFromState := make(map[string]string, len(b.Transitions))
	for _, t := range b.Transitions {
		if t.From != "" {
			transitionFromState[t.From] = t.On
		}
	}

	out := make([]executorActionView, 0, len(b.Actions))
	for _, a := range b.Actions {
		view := executorActionView{
			Name:        a.Name,
			GoName:      goExportFromUpperSnake(a.Name),
			ActionConst: "Action" + goExportFromUpperSnake(a.Name),
			Parameters:  append([]string(nil), a.RequiredParameters...),
			Risk:        riskGoConst(a.Risk),
			Approval:    approvalGoConst(a.ApprovalRequired),
		}
		for _, s := range a.AllowedStates {
			view.AllowedStates = append(view.AllowedStates, "State"+goExportFromUpperSnake(s))
		}
		for _, p := range a.RequiredPermissions {
			view.PermissionConsts = append(view.PermissionConsts, "Perm"+goExportFromPerm(p))
		}
		// Follow-up event: pick the transition that exits the first
		// allowed state. If no such transition exists, the executor
		// emits no follow-up event (the template branches on this).
		if len(a.AllowedStates) > 0 {
			if e, ok := transitionFromState[a.AllowedStates[0]]; ok {
				view.FollowUpEvent = "Event" + goExportFromUpperSnake(e)
			}
		}
		out = append(out, view)
	}
	return out
}

// riskGoConst maps a Blueprint risk string to the framework's
// risk constant.
func riskGoConst(risk string) string {
	switch risk {
	case "low":
		return "action.RiskLow"
	case "medium":
		return "action.RiskMedium"
	case "high":
		return "action.RiskHigh"
	case "critical":
		return "action.RiskCritical"
	default:
		return "action.RiskLow"
	}
}

// approvalGoConst maps the boolean to the framework's approval
// requirement constant.
func approvalGoConst(required bool) string {
	if required {
		return "action.ApprovalRequired"
	}
	return "action.ApprovalNever"
}

// joinComma joins strings with ", " for template rendering.
// Used by template helpers to render slice-shaped fields like
// AllowedStates and RequiredPermissions inline.
var executorFuncs = template.FuncMap{
	"joinComma": func(items []string) string {
		return strings.Join(items, ", ")
	},
	"joinCommaQuoted": func(items []string) string {
		out := make([]string, len(items))
		for i, s := range items {
			out[i] = fmt.Sprintf("%q", s)
		}
		return strings.Join(out, ", ")
	},
	// unexport lowercases the first rune so a PascalCase Go name
	// like "IssueRefund" becomes "issueRefund" — the conventional
	// name for a per-action contract factory function (see
	// agentic-ops/internal/domain/refund.go's `markPaidContract`).
	"unexport": func(s string) string {
		if s == "" {
			return s
		}
		first := s[0]
		if first >= 'A' && first <= 'Z' {
			return string(first+('a'-'A')) + s[1:]
		}
		return s
	},
	// adapterIdent turns a hyphenated domain name into the
	// PascalCase identifier used in the constants block's
	// AdapterX const. "refund-flow" → "RefundFlow".
	"adapterIdent": goExportName,
}

// executorTemplate is the text/template source for the executor
// stub file. Indentation is tabs; the framework's gofmt is
// strict and the output must round-trip cleanly through gofmt.
const executorTemplate = `// Package {{.PackageName}} — action contracts and executors for the
// {{.Domain}} domain. Generated by kiff-studio.
//
// Each contract factory below mirrors the convention from
// docs/conventions.md §"Action contracts":
//   Name, AllowedStates, RequiredParameters, RequiredPermissions,
//   Risk, ApprovalRequirement, Executor — in that order.
//
// Executor bodies are stubs. Each one returns a successful
// ActionResult and emits one follow-up event matching the
// post-execution state transition. Replace the stub body with
// your real side-effect logic; keep the FollowUpEvents shape so
// the runtime advances state correctly.
package {{.PackageName}}

import (
	"context"
	"fmt"
	"time"

	"github.com/kiff/kiff/pkg/kiff/action"
	"github.com/kiff/kiff/pkg/kiff/adapter"
	kiffdomain "github.com/kiff/kiff/pkg/kiff/domain"
	"github.com/kiff/kiff/pkg/kiff/event"
	"github.com/kiff/kiff/pkg/kiff/permission"
	"github.com/kiff/kiff/pkg/kiff/runtime"
	"github.com/kiff/kiff/pkg/kiff/store"
)

// Contracts returns the domain's action contracts. NewDefinition
// iterates over the result to register each contract on the
// runtime's action catalog.
func Contracts() []action.ActionContract {
	return []action.ActionContract{
{{- range .Actions}}
		{{.GoName | unexport}}Contract(),
{{- end}}
	}
}

// NewDefinition assembles the domain definition from the constants
// block and Contracts(). The framework's domain.Builder validates
// the result.
func NewDefinition() (kiffdomain.Definition, error) {
	b := kiffdomain.New("{{.Domain}}").
		Entity(Entity{{.Entity}}){{range .Events}}.
		Event({{.}}){{end}}{{range .Transitions}}.
		Transition({{.OnConst}}, {{.FromConst}}, {{.ToConst}}){{end}}{{range .Allows}}.
		Allow({{.StateConst}}, {{.ActionConst}}){{end}}
	for _, c := range Contracts() {
		b = b.Action(c)
	}
	return b.Build()
}

// NewPermissionPolicy returns the domain's permission policy. The
// solo-signup default (RFC 011 M1.2a) binds a tenant_owner role
// to every action's proposing + .approve permission. Replace this
// with your own role mapping when you split proposer / approver
// across humans.
func NewPermissionPolicy() *permission.SimplePolicy {
	policy := permission.NewSimplePolicy()
{{range .Roles}}{{- $role := .NameQuoted}}{{range .PermConsts}}	policy.GrantRole({{$role}}, {{.}})
{{end}}{{end -}}
	return policy
}

// NewInputAdapter creates the domain's passthrough adapter.
func NewInputAdapter() (adapter.Adapter, error) {
	return adapter.NewPassthroughAdapter(Adapter{{.AdapterIdent}})
}

// NewRuntime returns a runtime wired with in-memory stores.
func NewRuntime() (*runtime.Runtime, error) {
	return NewRuntimeWithStores(nil)
}

// NewRuntimeWithStores returns a runtime wired with the provided
// store bundle. A nil bundle falls back to in-memory stores.
func NewRuntimeWithStores(stores *store.Bundle) (*runtime.Runtime, error) {
	def, err := NewDefinition()
	if err != nil {
		return nil, err
	}
	in, err := NewInputAdapter()
	if err != nil {
		return nil, err
	}
	return runtime.NewForDomain(def, runtime.Config{
		PermissionPolicy: NewPermissionPolicy(),
		Adapters:         []adapter.Adapter{in},
		Stores:           stores,
	})
}

{{range .Actions}}
// {{.GoName | unexport}}Contract returns the {{.Name}} action contract.
func {{.GoName | unexport}}Contract() action.ActionContract {
	return action.ActionContract{
		Name:                {{.ActionConst}},
		AllowedStates:       []string{ {{joinComma .AllowedStates}} },
		RequiredParameters:  []string{ {{joinCommaQuoted .Parameters}} },
		RequiredPermissions: []permission.Permission{ {{joinComma .PermissionConsts}} },
		Risk:                {{.Risk}},
		ApprovalRequirement: {{.Approval}},
		Executor: func(_ context.Context, ctx action.ActionContext) (action.ActionResult, error) {
			// TODO: replace this stub with your real side effects.
			// Read parameters via ctx.Parameters[...]; emit
			// follow-up events to advance state.
			return action.ActionResult{
				ActionName:     {{.ActionConst}},
				EntityID:       ctx.EntityID,
				Status:         action.ExecutionSucceeded,
				Executed:       true,
				Message:        fmt.Sprintf("{{.Name}} executed for %s", ctx.EntityID),
				EffectsSummary: "{{.Name}} stub executed",
				FollowUpEvents: []event.Event{
{{- if .FollowUpEvent}}
					{
						ID:         fmt.Sprintf("evt-%s-%s-%d", {{.FollowUpEvent}}, ctx.EntityID, time.Now().UnixNano()),
						Type:       {{.FollowUpEvent}},
						EntityID:   ctx.EntityID,
						EntityType: Entity{{$.Entity}},
						Source:     "{{$.Domain}}/executor",
						ActorID:    ctx.Actor.ID,
						OccurredAt: time.Now().UTC(),
					},
{{- end}}
				},
				ExecutedAt: time.Now().UTC(),
			}, nil
		},
	}
}
{{end}}`

// buildEventConstRefs returns the Go const ref for each event in
// the Blueprint's Events list. Used by the template's
// kiffdomain.New(...).Event(...) chain.
func buildEventConstRefs(b Blueprint) []string {
	out := make([]string, 0, len(b.Events))
	for _, e := range b.Events {
		out = append(out, "Event"+goExportFromUpperSnake(e))
	}
	return out
}

// buildTransitionViews resolves each transition's event/state
// names into the Go const refs the template emits inline.
// FromConst is `""` for the initial transition (the kiffdomain
// builder's `.Transition` accepts an empty string for that case).
func buildTransitionViews(b Blueprint) []executorTransitionView {
	out := make([]executorTransitionView, 0, len(b.Transitions))
	for _, t := range b.Transitions {
		view := executorTransitionView{
			OnConst: "Event" + goExportFromUpperSnake(t.On),
			ToConst: "State" + goExportFromUpperSnake(t.To),
		}
		if t.From == "" {
			view.FromConst = `""`
		} else {
			view.FromConst = "State" + goExportFromUpperSnake(t.From)
		}
		out = append(out, view)
	}
	return out
}

// buildAllowViews emits one Allow(state, action) registration per
// (state, action) pair where the action lists the state in
// AllowedStates. The state machine's "allowed actions" map is the
// reverse of the action's AllowedStates list; this helper does
// the inversion deterministically.
func buildAllowViews(b Blueprint) []executorAllowView {
	var out []executorAllowView
	for _, a := range b.Actions {
		actionConst := "Action" + goExportFromUpperSnake(a.Name)
		for _, s := range a.AllowedStates {
			out = append(out, executorAllowView{
				StateConst:  "State" + goExportFromUpperSnake(s),
				ActionConst: actionConst,
			})
		}
	}
	return out
}

// buildRoleViews translates Blueprint.Roles into the template's
// per-role view, with role names sorted lexicographically for
// deterministic output and permission strings resolved to their
// constants-block const refs.
func buildRoleViews(b Blueprint) []executorRoleView {
	if len(b.Roles) == 0 {
		return nil
	}
	roleNames := make([]string, 0, len(b.Roles))
	for r := range b.Roles {
		roleNames = append(roleNames, r)
	}
	sort.Strings(roleNames)
	out := make([]executorRoleView, 0, len(roleNames))
	for _, r := range roleNames {
		view := executorRoleView{NameQuoted: fmt.Sprintf("%q", r)}
		for _, p := range b.Roles[r] {
			view.PermConsts = append(view.PermConsts, "Perm"+goExportFromPerm(p))
		}
		out = append(out, view)
	}
	return out
}
