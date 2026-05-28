package studio

import (
	"sort"
	"strings"
	"testing"
)

// TestBuildScaffold_ProducesAllExpectedFiles asserts the scaffold
// covers the file set RFC 012 names: go.mod, cmd/server/main.go,
// the domain constants, the executor stub, the test scaffold,
// and the kiff.yaml. The kiff CLI's `kiff new` template tree is
// the reference shape; the cloud's "download" path renders these
// files verbatim.
func TestBuildScaffold_ProducesAllExpectedFiles(t *testing.T) {
	t.Parallel()
	s, err := BuildScaffold(canonicalRefundBlueprint(), "github.com/acme/orders")
	if err != nil {
		t.Fatalf("BuildScaffold: %v", err)
	}
	want := []string{
		"go.mod",
		"cmd/server/main.go",
		"internal/domain/refundflow.go",
		"internal/domain/refundflow_constants.go",
		"internal/domain/refundflow_test.go",
		"internal/domain/kiff.yaml",
	}
	got := keys(s.Files)
	if !equalUnordered(got, want) {
		t.Fatalf("file set mismatch\n got: %v\nwant: %v", got, want)
	}
}

// TestBuildScaffold_GoModCarriesModulePath confirms the user's
// module path lands in the rendered go.mod verbatim.
func TestBuildScaffold_GoModCarriesModulePath(t *testing.T) {
	t.Parallel()
	s, err := BuildScaffold(canonicalRefundBlueprint(), "github.com/acme/orders")
	if err != nil {
		t.Fatalf("BuildScaffold: %v", err)
	}
	out := string(s.Files["go.mod"])
	if !strings.Contains(out, "module github.com/acme/orders") {
		t.Fatalf("go.mod missing module path:\n%s", out)
	}
	if !strings.Contains(out, "github.com/kiffhq/kiff v0.1.0") {
		t.Fatalf("go.mod missing framework dep:\n%s", out)
	}
}

// TestBuildScaffold_MainImportsDomainPackage confirms the
// scaffold's cmd/server/main.go imports the user's domain
// package at the right module-relative path.
func TestBuildScaffold_MainImportsDomainPackage(t *testing.T) {
	t.Parallel()
	s, err := BuildScaffold(canonicalRefundBlueprint(), "github.com/acme/orders")
	if err != nil {
		t.Fatalf("BuildScaffold: %v", err)
	}
	out := string(s.Files["cmd/server/main.go"])
	if !strings.Contains(out, `"github.com/acme/orders/internal/domain"`) {
		t.Fatalf("main.go missing domain import:\n%s", out)
	}
	if !strings.Contains(out, "domain.NewRuntime()") {
		t.Fatalf("main.go does not call domain.NewRuntime:\n%s", out)
	}
}

// TestBuildScaffold_RejectsEmptyModulePath asserts the scaffold
// requires a module path; the rendered go.mod would be malformed
// without one.
func TestBuildScaffold_RejectsEmptyModulePath(t *testing.T) {
	t.Parallel()
	_, err := BuildScaffold(canonicalRefundBlueprint(), "")
	if err == nil || !strings.Contains(err.Error(), "module path is required") {
		t.Fatalf("expected module-path error, got %v", err)
	}
}

func keys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func equalUnordered(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	sa := append([]string(nil), a...)
	sb := append([]string(nil), b...)
	sort.Strings(sa)
	sort.Strings(sb)
	for i := range sa {
		if sa[i] != sb[i] {
			return false
		}
	}
	return true
}
