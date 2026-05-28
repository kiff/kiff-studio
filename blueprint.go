// Package studio is the KIFF authoring layer.
//
// Studio takes user-shaped input — a problem description, a URL, an
// existing-code snippet, or a structured form — and produces a
// Blueprint: the events, states, actions, permissions, and audit
// hooks of a KIFF domain. From a Blueprint the package renders
// convention-faithful artifacts: a kiff.yaml the framework's
// domain validator accepts on the first try, a Go constants block
// matching the framework's naming conventions, an executor stub
// per action, a test scaffold with the three convention-required
// cases (happy path, blocked path, replay), and a full kiff
// new-shaped scaffold the user can paste into a new app.
//
// The package has three rules (RFC 012):
//
//  1. Studio produces; it does not host. The package returns
//     bytes and structured values. It does not call into any
//     KIFF runtime.
//  2. Studio's output is convention-faithful. Every artifact
//     parses against the framework's domain validator and
//     follows the framework's conventions for naming and
//     structure.
//  3. Studio depends on the framework, not on the cloud. The
//     dependency is one-way.
//
// Studio v0.1's load-bearing surface is blueprint generation
// (RFC 012 amendment 2026-05-28). Deterministic builders remain
// — they are the rendering half of the package, downstream of
// generation.
//
// The primary entry point is GenerateBlueprint, which routes the
// caller's Input to the right generator (deterministic for
// BuilderInput, LLM-assisted for the description/URL/code shapes)
// and returns a validated Blueprint. Consumers then call
// BuildYAML, BuildConstants, BuildExecutorStub, BuildTestScaffold,
// or BuildScaffold on that Blueprint to render the artifacts they
// need.
package studio

import (
	"errors"
	"fmt"
)

// Blueprint is the structured authoring shape Studio produces.
// It is the cross-section of "everything a KIFF domain needs to
// be wired" expressed as pure data: events, states, transitions,
// actions, permissions. The shape mirrors the YAML the framework's
// domain.ParseAndBuild accepts (see apps/api/internal/domain/spec.go
// in kiff-cloud), but with package-public Go types.
//
// A Blueprint is the canonical intermediate between an Input and
// any rendered artifact. All artifact builders take a Blueprint;
// no builder takes an Input directly.
type Blueprint struct {
	// Domain is the workflow name. Lowercase, hyphenated.
	// Maps to the YAML's top-level `domain` field.
	Domain string

	// Entity is the entity type. PascalCase.
	// Maps to `entity`.
	Entity string

	// Events is the full list of event types the domain emits.
	// UPPER_SNAKE_CASE. Each entry is unique.
	Events []string

	// States is the full list of states the entity may occupy.
	// UPPER_SNAKE_CASE. Each entry is unique.
	States []string

	// Transitions describe how events advance the state machine.
	// Each transition names the triggering event, the from-state
	// (empty string for the initial transition), and the to-state.
	Transitions []Transition

	// Actions are the action contracts the domain exposes. Each
	// action has its own allowed states, parameters, permissions,
	// risk, approval requirement, and executor key.
	Actions []Action

	// Roles maps role names to the dotted.lowercase permissions
	// each role holds. The canonical solo-signup shape (RFC 011
	// M1.2a) has one tenant_owner role holding both the
	// proposing and the .approve permissions for every action.
	Roles map[string][]string
}

// Transition is one rule in the domain's state machine.
type Transition struct {
	On   string // event name (UPPER_SNAKE_CASE)
	From string // empty string = initial transition
	To   string // target state (UPPER_SNAKE_CASE)
}

// Action is one action contract on the domain. The fields mirror
// the framework's action.ActionContract, expressed as plain data
// so consumers don't import the framework just to read a Blueprint.
type Action struct {
	// Name is the action name, UPPER_SNAKE_CASE.
	Name string

	// AllowedStates are the states in which the action may be
	// proposed and executed.
	AllowedStates []string

	// RequiredParameters are the parameter names callers must
	// provide. snake_case.
	RequiredParameters []string

	// RequiredPermissions are the permissions a caller's roles
	// must satisfy. dotted.lowercase. The canonical shape
	// (RFC 011 M1.2a) lists both <workflow>.<action> (proposing)
	// and <workflow>.<action>.approve (granting) for any action
	// that requires approval.
	RequiredPermissions []string

	// Risk is one of "low", "medium", "high", "critical".
	Risk string

	// ApprovalRequired flips the action between "no approval
	// gate" and "framework's runtime.RequestApproval +
	// ReviewApproval gate." High-risk actions should set this
	// true.
	ApprovalRequired bool

	// Executor is the executor key the framework's runtime uses
	// to dispatch execution. Cloud sets this to "cloud.proxy"
	// for tenant-saved domains; Framework-only adopters set
	// their own executor name. The package does not default this
	// field — it is generator-driven.
	Executor string
}

// Validate returns nil when the Blueprint is structurally
// well-formed and convention-faithful. It is the single check the
// rendering builders run before producing artifacts; a Blueprint
// that passes Validate produces YAML the framework's
// domain.ParseAndBuild accepts.
//
// Rules enforced (in order, first failure wins):
//
//   - Domain is non-empty, lowercase + hyphens, starts with a letter.
//   - Entity is non-empty, PascalCase.
//   - At least two states.
//   - At least one action.
//   - State / event / role names follow their conventions.
//   - Each action has all seven contract fields populated:
//     Name, AllowedStates (>=1), RequiredParameters (may be
//     empty), RequiredPermissions (>=1), Risk, ApprovalRequired,
//     Executor.
//   - Each transition's On is in Events; From is empty or in
//     States; To is in States.
//   - Each role's permissions are dotted.lowercase.
//
// Validate does not check semantic invariants (e.g. that the
// state machine is reachable, that every event has a transition).
// Those land in the framework's domain.ParseAndBuild.
func (b Blueprint) Validate() error {
	if !isLowerHyphen(b.Domain) {
		return fmt.Errorf("blueprint: domain %q must be lowercase letters, digits, and hyphens (e.g. refund-flow)", b.Domain)
	}
	if !isPascalCase(b.Entity) {
		return fmt.Errorf("blueprint: entity %q must be PascalCase (e.g. Order)", b.Entity)
	}
	if len(b.States) < 2 {
		return errors.New("blueprint: at least two states are required (a before-state and an after-state)")
	}
	if len(b.Actions) == 0 {
		return errors.New("blueprint: at least one action is required")
	}

	stateSet := make(map[string]struct{}, len(b.States))
	for _, s := range b.States {
		if !isUpperSnake(s) {
			return fmt.Errorf("blueprint: state %q must be UPPER_SNAKE_CASE", s)
		}
		if _, dup := stateSet[s]; dup {
			return fmt.Errorf("blueprint: duplicate state %q", s)
		}
		stateSet[s] = struct{}{}
	}

	eventSet := make(map[string]struct{}, len(b.Events))
	for _, e := range b.Events {
		if !isUpperSnake(e) {
			return fmt.Errorf("blueprint: event %q must be UPPER_SNAKE_CASE", e)
		}
		if _, dup := eventSet[e]; dup {
			return fmt.Errorf("blueprint: duplicate event %q", e)
		}
		eventSet[e] = struct{}{}
	}

	for _, t := range b.Transitions {
		if _, ok := eventSet[t.On]; !ok {
			return fmt.Errorf("blueprint: transition.on %q is not declared in events", t.On)
		}
		if t.From != "" {
			if _, ok := stateSet[t.From]; !ok {
				return fmt.Errorf("blueprint: transition.from %q is not declared in states", t.From)
			}
		}
		if _, ok := stateSet[t.To]; !ok {
			return fmt.Errorf("blueprint: transition.to %q is not declared in states", t.To)
		}
	}

	for i, a := range b.Actions {
		if err := a.validate(stateSet); err != nil {
			return fmt.Errorf("blueprint: action[%d] %q: %w", i, a.Name, err)
		}
	}

	for role, perms := range b.Roles {
		if !isSnakeCase(role) {
			return fmt.Errorf("blueprint: role %q must be snake_case", role)
		}
		for _, p := range perms {
			if !isDottedLower(p) {
				return fmt.Errorf("blueprint: role %q permission %q must be dotted.lowercase", role, p)
			}
		}
	}

	return nil
}

func (a Action) validate(states map[string]struct{}) error {
	if !isUpperSnake(a.Name) {
		return fmt.Errorf("name %q must be UPPER_SNAKE_CASE", a.Name)
	}
	if len(a.AllowedStates) == 0 {
		return errors.New("allowed_states must list at least one state")
	}
	for _, s := range a.AllowedStates {
		if _, ok := states[s]; !ok {
			return fmt.Errorf("allowed_state %q is not declared in states", s)
		}
	}
	for _, p := range a.RequiredParameters {
		if !isSnakeCase(p) {
			return fmt.Errorf("required_parameter %q must be snake_case", p)
		}
	}
	if len(a.RequiredPermissions) == 0 {
		return errors.New("required_permissions must list at least one permission")
	}
	for _, p := range a.RequiredPermissions {
		if !isDottedLower(p) {
			return fmt.Errorf("required_permission %q must be dotted.lowercase", p)
		}
	}
	switch a.Risk {
	case "low", "medium", "high", "critical":
	default:
		return fmt.Errorf("risk %q must be one of low, medium, high, critical", a.Risk)
	}
	if a.Executor == "" {
		return errors.New("executor key is required (the generator must set it)")
	}
	return nil
}
