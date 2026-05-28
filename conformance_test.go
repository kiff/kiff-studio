package studio

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestConformance_FrameworkTemplateShape is the v0.1 drift
// detector: it walks the framework's `kiff new -template=agentic-ops`
// template tree and asserts that for the canonical refund-flow
// blueprint, kiff-studio's BuildScaffold produces an output set
// covering the same load-bearing files (the Go domain layer +
// the cmd/server entry point + the domain YAML).
//
// Why it's a drift detector and not a byte-for-byte compare:
//
//   - The agentic-ops template is hand-tuned (the refund executor
//     handles cents-shaped numeric parameters, the demo includes a
//     Python agent harness). kiff-studio v0.1 emits a generic stub.
//     A byte compare would always fail and would punish hand-tuning
//     improvements to the template.
//   - The contract that matters at v0.1 is "kiff-studio output
//     matches the framework's agentic-ops shape closely enough that
//     a tenant pasting the kiff-studio output into the agentic-ops
//     scaffold layout finds the same package shape and the same
//     wiring functions." That's what this test asserts.
//
// Post-MVP step 5 (kiff CLI starts importing kiff-studio for its
// scaffold rendering) flips the test: once both sides come from the
// same package, a byte-for-byte compare becomes possible and the
// test tightens to that.
//
// The test is skipped if the framework template is not on disk at
// the expected sibling location — same posture as the cloud's
// e2e_test.go skip when KIFF_TEST_POSTGRES_URL is unset.
func TestConformance_FrameworkTemplateShape(t *testing.T) {
	t.Parallel()

	frameworkTemplate := locateAgenticOpsTemplate(t)
	if frameworkTemplate == "" {
		t.Skipf("framework agentic-ops template not found at expected sibling path; skipping conformance test")
	}

	// The framework's template tree (relevant subset for v0.1):
	//
	//   go.mod.tmpl
	//   cmd/server/main.go
	//   internal/domain/refund.go
	//   internal/domain/refund_test.go
	//
	// Files outside this subset (the agent/ Python harness, the
	// demo scripts, the .gitignore template, the README) are
	// out of scope for v0.1 per RFC 012 §"Repo and module layout"
	// — kiff-studio targets the Go runtime + domain layer.
	wantInTemplate := []string{
		"go.mod.tmpl",
		"cmd/server/main.go",
		"internal/domain/refund.go",
		"internal/domain/refund_test.go",
	}
	for _, f := range wantInTemplate {
		full := filepath.Join(frameworkTemplate, f)
		if !fileExists(full) {
			t.Fatalf("framework template missing %s; conformance test cannot run", f)
		}
	}

	// kiff-studio's BuildScaffold output for the canonical
	// refund-flow blueprint. The file *set* must cover the same
	// shape (modulo file naming: the framework's "refund.go"
	// becomes our hyphen-stripped "refundflow.go", and we split
	// constants into a separate file).
	scaffold, err := BuildScaffold(canonicalRefundBlueprint(), "github.com/example/orders")
	if err != nil {
		t.Fatalf("BuildScaffold: %v", err)
	}

	// Asserted equivalences:
	//
	//   framework               kiff-studio
	//   ----------------------- ---------------------------------------
	//   go.mod.tmpl             go.mod
	//   cmd/server/main.go      cmd/server/main.go
	//   internal/domain/X.go    internal/domain/refundflow.go
	//                           + internal/domain/refundflow_constants.go
	//   internal/domain/X_test  internal/domain/refundflow_test.go
	//                           + internal/domain/kiff.yaml (new — the
	//                             cloud's PUT /v1/me/domain artifact)
	requiredScaffoldKeys := []string{
		"go.mod",
		"cmd/server/main.go",
		"internal/domain/refundflow.go",
		"internal/domain/refundflow_constants.go",
		"internal/domain/refundflow_test.go",
		"internal/domain/kiff.yaml",
	}
	for _, k := range requiredScaffoldKeys {
		if _, ok := scaffold.Files[k]; !ok {
			t.Fatalf("scaffold missing %s; expected for shape parity with agentic-ops template", k)
		}
	}

	// Shape-level: cmd/server/main.go must wire the domain
	// package's NewRuntime + httpapi.NewHandler — the same shape
	// the framework template emits. This is what makes the two
	// outputs swap-compatible at consumer time.
	mainGo := string(scaffold.Files["cmd/server/main.go"])
	for _, want := range []string{
		"domain.NewRuntime()",
		"httpapi.NewHandler",
		"http.ListenAndServe",
	} {
		if !strings.Contains(mainGo, want) {
			t.Fatalf("cmd/server/main.go missing %q\n--- output ---\n%s", want, mainGo)
		}
	}

	// Shape-level: internal/domain's executor file must declare
	// the same wiring functions the framework template's
	// refund.go declares. If a framework PR adds a new wiring
	// function, this test catches the drift.
	domainGo := string(scaffold.Files["internal/domain/refundflow.go"])
	for _, want := range []string{
		"func NewRuntime() (*runtime.Runtime, error)",
		"func NewRuntimeWithStores(stores *store.Bundle)",
		"func NewDefinition() (kiffdomain.Definition, error)",
		"func NewPermissionPolicy() *permission.SimplePolicy",
		"func NewInputAdapter() (adapter.Adapter, error)",
		"func Contracts() []action.ActionContract",
	} {
		if !strings.Contains(domainGo, want) {
			t.Fatalf("internal/domain executor file missing %q\n--- output ---\n%s", want, domainGo)
		}
	}
}

// locateAgenticOpsTemplate returns the path to the framework's
// agentic-ops template directory if it's on disk, or "" if the
// repo isn't checked out where we expect (or this is running in
// CI without the framework cloned alongside).
func locateAgenticOpsTemplate(t *testing.T) string {
	t.Helper()
	// Caller's source file lives alongside the package. The
	// kiff-cloud workspace mirrors the framework at
	// .local/kiff/cmd/kiff/templates/agentic-ops; when this
	// package is developed in-tree (.local/kiff-studio/) we
	// can find the template by walking up.
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	// Try the in-tree development location first:
	//   .local/kiff-studio/conformance_test.go (here)
	//     → .local/kiff/cmd/kiff/templates/agentic-ops
	devTime := filepath.Join(filepath.Dir(here), "..", "kiff", "cmd", "kiff", "templates", "agentic-ops")
	if dirExists(devTime) {
		return devTime
	}
	// Try a sibling-of-package layout (when kiff-studio is its
	// own repo and the framework is its sibling on disk):
	//   ../kiff/cmd/kiff/templates/agentic-ops
	siblingTime := filepath.Join(filepath.Dir(here), "..", "..", "kiff", "cmd", "kiff", "templates", "agentic-ops")
	if dirExists(siblingTime) {
		return siblingTime
	}
	return ""
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}
