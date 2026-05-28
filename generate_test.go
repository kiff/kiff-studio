package studio

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

// TestGenerateBlueprint_BuilderInput_ProducesCanonicalBlueprint
// exercises the deterministic path. A BuilderInput shaped like
// the embedded Studio v0 form (post-#203) should produce the
// canonical refund-flow Blueprint byte-for-byte.
func TestGenerateBlueprint_BuilderInput_ProducesCanonicalBlueprint(t *testing.T) {
	t.Parallel()
	in := canonicalRefundInput()
	got, err := GenerateBlueprint(context.Background(), in)
	if err != nil {
		t.Fatalf("GenerateBlueprint: %v", err)
	}
	want := canonicalRefundBlueprint()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("blueprint mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

// TestGenerateBlueprint_BuilderInput_IsDeterministic asserts that
// the same BuilderInput produces a structurally identical
// Blueprint on repeated calls. The map field (Roles) is the only
// place a non-deterministic implementation could leak; the
// generator builds it from the action list, so identical inputs
// must yield identical maps.
func TestGenerateBlueprint_BuilderInput_IsDeterministic(t *testing.T) {
	t.Parallel()
	in := canonicalRefundInput()
	first, err := GenerateBlueprint(context.Background(), in)
	if err != nil {
		t.Fatalf("GenerateBlueprint #1: %v", err)
	}
	second, err := GenerateBlueprint(context.Background(), in)
	if err != nil {
		t.Fatalf("GenerateBlueprint #2: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("non-deterministic generation\n#1: %#v\n#2: %#v", first, second)
	}
}

// TestGenerateBlueprint_DescriptionInput_RequiresLLMGenerator
// asserts that the LLM-assisted shapes return
// ErrLLMGeneratorRequired when no generator is wired. Studio
// v0.1 ships the deterministic path only; the LLM path is wired
// by the consumer (Cloud's wrapper, framework-only adopters).
func TestGenerateBlueprint_DescriptionInput_RequiresLLMGenerator(t *testing.T) {
	t.Parallel()
	_, err := GenerateBlueprint(context.Background(), DescriptionInput{Text: "a refund flow"})
	if !errors.Is(err, ErrLLMGeneratorRequired) {
		t.Fatalf("got %v, want ErrLLMGeneratorRequired", err)
	}
}

// TestGenerateBlueprint_DescriptionInput_DispatchesToGenerator
// confirms that when an LLMGenerator is wired, the input lands
// at it and the returned Blueprint flows back through validation.
func TestGenerateBlueprint_DescriptionInput_DispatchesToGenerator(t *testing.T) {
	t.Parallel()
	stub := &stubLLM{out: canonicalRefundBlueprint()}
	got, err := GenerateBlueprint(
		context.Background(),
		DescriptionInput{Text: "a refund flow"},
		WithLLMGenerator(stub),
	)
	if err != nil {
		t.Fatalf("GenerateBlueprint: %v", err)
	}
	if stub.calls != 1 {
		t.Fatalf("LLM Generate called %d times; want 1", stub.calls)
	}
	if got.Domain != "refund-flow" {
		t.Fatalf("got domain %q; want refund-flow", got.Domain)
	}
}

// TestGenerateBlueprint_RejectsInvalidGeneratorOutput asserts
// that an LLMGenerator returning a Blueprint that fails
// Validate produces ErrInvalidGeneration. The generator is a
// trust boundary; bad output is the package's responsibility to
// catch.
func TestGenerateBlueprint_RejectsInvalidGeneratorOutput(t *testing.T) {
	t.Parallel()
	bad := canonicalRefundBlueprint()
	bad.Domain = "BadName" // forces Validate to fail
	stub := &stubLLM{out: bad}
	_, err := GenerateBlueprint(
		context.Background(),
		DescriptionInput{Text: "x"},
		WithLLMGenerator(stub),
	)
	if !errors.Is(err, ErrInvalidGeneration) {
		t.Fatalf("got %v, want ErrInvalidGeneration wrap", err)
	}
}

// TestGenerateBlueprint_RejectsNilInput is a defence-in-depth
// check; the package's contract says input is required.
func TestGenerateBlueprint_RejectsNilInput(t *testing.T) {
	t.Parallel()
	_, err := GenerateBlueprint(context.Background(), nil)
	if err == nil {
		t.Fatalf("expected error on nil input")
	}
}

// canonicalRefundInput is the BuilderInput counterpart of
// canonicalRefundBlueprint — the structured-form representation
// of the same refund-flow domain. The deterministic generator
// turns this into canonicalRefundBlueprint exactly.
func canonicalRefundInput() BuilderInput {
	return BuilderInput{
		WorkflowName: "refund-flow",
		EntityType:   "Order",
		States:       []string{"READY_FOR_REFUND", "REFUNDED"},
		Actions: []ActionInput{
			{
				Name:               "ISSUE_REFUND",
				AllowedStates:      []string{"READY_FOR_REFUND"},
				RequiredParameters: []string{"amount", "reason"},
				ApprovalRequired:   true,
				Risk:               "high",
			},
		},
	}
}

// stubLLM is a minimal LLMGenerator for the dispatch tests.
type stubLLM struct {
	out   Blueprint
	calls int
}

func (s *stubLLM) Generate(_ context.Context, _ Input) (Blueprint, error) {
	s.calls++
	return s.out, nil
}
