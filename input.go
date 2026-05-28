package studio

import (
	"context"
	"errors"
)

// Input is the user-shaped authoring shape Studio accepts.
// Implementations are concrete shapes that GenerateBlueprint
// dispatches on. Four are defined in v0.1; future versions may
// add more without changing the GenerateBlueprint signature.
//
// The four v0.1 shapes are:
//
//   - DescriptionInput  — plain-English problem description
//   - URLInput          — a URL the package fetches and analyzes
//   - ExistingCodeInput — a code snippet or repo path
//   - BuilderInput      — the structured-form shape, carried
//     forward from apps/web/internal/onboarding/builder.go
//
// The first three route to an LLM-assisted generator (the
// generation backend reads the input, asks a model to produce a
// Blueprint, and validates the output). The fourth routes to a
// deterministic generator (the carry-forward of today's
// embedded Studio v0; no model call).
//
// Studio v0.1 ships with the deterministic generator only.
// Callers wire an LLM-assisted Generator at construction time
// (see WithLLMGenerator); without one, the LLM-assisted inputs
// return ErrLLMGeneratorRequired.
type Input interface {
	// inputKind returns a stable string identifying the input
	// shape. Used by GenerateBlueprint to dispatch and by
	// audit / log code that wants to record which shape fed a
	// generation.
	inputKind() string

	// validate returns nil when the input is structurally
	// well-formed for its shape. Each shape has its own rules.
	validate() error
}

// DescriptionInput is plain-English text that describes the
// workflow the user wants to author. The LLM-assisted generator
// reads it and produces a first Blueprint draft.
type DescriptionInput struct {
	// Text is the description. Required.
	Text string

	// Hints are optional structured nudges the user provides
	// alongside the description (e.g. "this is for a fintech
	// case workflow"). They flow into the prompt verbatim.
	Hints []string
}

func (DescriptionInput) inputKind() string { return "description" }

func (d DescriptionInput) validate() error {
	if d.Text == "" {
		return errors.New("description input: text is required")
	}
	return nil
}

// URLInput names a URL the package fetches and analyzes. The
// LLM-assisted generator extracts a workflow shape from the
// fetched content (a documentation page, an OpenAPI spec, a
// README).
type URLInput struct {
	// URL is the location to fetch. Required.
	URL string
}

func (URLInput) inputKind() string { return "url" }

func (u URLInput) validate() error {
	if u.URL == "" {
		return errors.New("url input: url is required")
	}
	return nil
}

// ExistingCodeInput names existing code to analyze for a
// blueprint. The LLM-assisted generator reads the code and
// extracts the implicit workflow (state transitions, side
// effects, decision points) into a Blueprint shape.
type ExistingCodeInput struct {
	// Snippet is a self-contained piece of code. Required if
	// RepoPath is empty.
	Snippet string

	// RepoPath is a filesystem path to a repo root. Required
	// if Snippet is empty.
	RepoPath string

	// Language is an optional hint ("go", "python", "ts", ...).
	// The generator infers it when empty.
	Language string
}

func (ExistingCodeInput) inputKind() string { return "existing_code" }

func (e ExistingCodeInput) validate() error {
	if e.Snippet == "" && e.RepoPath == "" {
		return errors.New("existing-code input: snippet or repo_path is required")
	}
	return nil
}

// BuilderInput is the structured-form authoring shape. It
// carries forward what apps/web/internal/onboarding/builder.go
// accepts today, extended with multi-action and multi-role
// support per RFC 011 §M2.
//
// Users who already know the KIFF vocabulary (or who have built
// a domain elsewhere) skip generation and pass a BuilderInput
// straight to the deterministic generator. The output is
// byte-deterministic for byte-identical input.
type BuilderInput struct {
	// WorkflowName is lowercase + hyphenated (e.g. "refund-flow").
	WorkflowName string

	// EntityType is PascalCase (e.g. "Order").
	EntityType string

	// States is the linear chain of states. The first state is
	// the "before" state where the first action is allowed; the
	// rest are reached after the actions run.
	States []string

	// Actions are the action contracts on this domain.
	Actions []ActionInput

	// Roles maps role names to the dotted.lowercase permissions
	// each role holds. When empty, the generator emits a single
	// tenant_owner role bound to every action's proposing
	// permission and its .approve counterpart (the canonical
	// solo-signup shape from RFC 011 M1.2a).
	Roles []RoleInput
}

// ActionInput is one action in a BuilderInput. Mirrors the
// fields the framework's action.ActionContract carries.
type ActionInput struct {
	// Name is the action name (UPPER_SNAKE_CASE).
	Name string

	// AllowedStates lists the states in which the action may
	// be proposed. UPPER_SNAKE_CASE; each entry must appear in
	// BuilderInput.States.
	AllowedStates []string

	// Transitions describe how the action's execution advances
	// the state machine. Each transition names the triggering
	// event, the from-state (empty string for the initial
	// transition), and the to-state. When empty, the
	// deterministic generator falls back to linear inference
	// (the legacy single-action behavior).
	Transitions []TransitionInput

	// RequiredParameters are the parameter names callers must
	// provide. snake_case.
	RequiredParameters []string

	// ApprovalRequired flips the action's risk and approval
	// fields. true → risk=high, approval=required.
	ApprovalRequired bool

	// Risk is one of "low", "medium", "high", "critical". Empty
	// is allowed; the generator derives it from
	// ApprovalRequired.
	Risk string
}

// TransitionInput is one transition rule on a BuilderInput
// action.
type TransitionInput struct {
	On   string // event name
	From string // empty string = initial transition
	To   string
}

// RoleInput is one role in a BuilderInput. The generator emits
// the role with the listed permissions verbatim.
type RoleInput struct {
	Name        string
	Permissions []string
}

func (BuilderInput) inputKind() string { return "builder" }

func (b BuilderInput) validate() error {
	if !isLowerHyphen(b.WorkflowName) {
		return errors.New("builder input: workflow_name must be lowercase letters, digits, and hyphens (e.g. refund-flow)")
	}
	if !isPascalCase(b.EntityType) {
		return errors.New("builder input: entity_type must be PascalCase (e.g. Order)")
	}
	if len(b.States) < 2 {
		return errors.New("builder input: at least two states are required")
	}
	for _, s := range b.States {
		if !isUpperSnake(s) {
			return errors.New("builder input: state " + s + " must be UPPER_SNAKE_CASE")
		}
	}
	if len(b.Actions) == 0 {
		return errors.New("builder input: at least one action is required")
	}
	for _, a := range b.Actions {
		if !isUpperSnake(a.Name) {
			return errors.New("builder input: action name " + a.Name + " must be UPPER_SNAKE_CASE")
		}
		for _, s := range a.AllowedStates {
			if !isUpperSnake(s) {
				return errors.New("builder input: action " + a.Name + " allowed_state " + s + " must be UPPER_SNAKE_CASE")
			}
		}
		for _, p := range a.RequiredParameters {
			if !isSnakeCase(p) {
				return errors.New("builder input: action " + a.Name + " parameter " + p + " must be snake_case")
			}
		}
	}
	for _, r := range b.Roles {
		if !isSnakeCase(r.Name) {
			return errors.New("builder input: role name " + r.Name + " must be snake_case")
		}
		for _, p := range r.Permissions {
			if !isDottedLower(p) {
				return errors.New("builder input: role " + r.Name + " permission " + p + " must be dotted.lowercase")
			}
		}
	}
	return nil
}

// LLMGenerator is the contract the LLM-assisted input shapes
// (description, URL, existing code) dispatch through. Studio
// does not provide a concrete implementation in v0.1; consumers
// wire one in (Cloud's wrapper in #211, Framework-only adopters
// bring their own). Tests use a stub.
//
// Generate returns a Blueprint that must pass Blueprint.Validate
// before GenerateBlueprint hands it back to the caller. A
// generator that returns an invalid Blueprint is a bug; the
// caller-side path treats it as ErrInvalidGeneration.
type LLMGenerator interface {
	Generate(ctx context.Context, input Input) (Blueprint, error)
}

// ErrLLMGeneratorRequired is returned when GenerateBlueprint
// receives a DescriptionInput, URLInput, or ExistingCodeInput
// and no LLMGenerator was wired. The deterministic path
// (BuilderInput) does not produce this error.
var ErrLLMGeneratorRequired = errors.New("studio: LLM generator is required for description / url / existing-code inputs")

// ErrInvalidGeneration is returned when an LLMGenerator returns
// a Blueprint that fails Blueprint.Validate. The wrapped error
// is the validation failure.
var ErrInvalidGeneration = errors.New("studio: generated blueprint failed validation")

// Option configures GenerateBlueprint and its surface. The
// pattern keeps the package's primary entry point a single
// function call while leaving room for v0.2 to add knobs
// (timeouts, model selection, audit hooks) without breaking the
// signature.
type Option func(*options)

type options struct {
	llm LLMGenerator
}

// WithLLMGenerator wires an LLM-assisted generator into
// GenerateBlueprint. Without it, the description / URL /
// existing-code input shapes return ErrLLMGeneratorRequired.
func WithLLMGenerator(g LLMGenerator) Option {
	return func(o *options) { o.llm = g }
}

// GenerateBlueprint is the package's primary entry point. It
// validates the input, routes it to the right generator
// (deterministic for BuilderInput, LLM-assisted for the rest),
// validates the resulting Blueprint, and returns it.
//
// The LLM-assisted path requires an LLMGenerator wired through
// WithLLMGenerator; without one, the LLM-assisted inputs return
// ErrLLMGeneratorRequired. The deterministic path has no
// external dependency.
//
// The returned Blueprint is always Validate-clean. A consumer
// can pass it to BuildYAML, BuildConstants, BuildExecutorStub,
// BuildTestScaffold, or BuildScaffold without re-validating.
func GenerateBlueprint(ctx context.Context, input Input, opts ...Option) (Blueprint, error) {
	if input == nil {
		return Blueprint{}, errors.New("studio: input is required")
	}
	if err := input.validate(); err != nil {
		return Blueprint{}, err
	}

	o := options{}
	for _, opt := range opts {
		opt(&o)
	}

	switch in := input.(type) {
	case BuilderInput:
		bp := generateFromBuilder(in)
		if err := bp.Validate(); err != nil {
			return Blueprint{}, err
		}
		return bp, nil
	case DescriptionInput, URLInput, ExistingCodeInput:
		if o.llm == nil {
			return Blueprint{}, ErrLLMGeneratorRequired
		}
		bp, err := o.llm.Generate(ctx, input)
		if err != nil {
			return Blueprint{}, err
		}
		if err := bp.Validate(); err != nil {
			return Blueprint{}, errors.Join(ErrInvalidGeneration, err)
		}
		return bp, nil
	default:
		return Blueprint{}, errors.New("studio: unknown input shape")
	}
}
