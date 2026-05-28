# kiff-studio

The KIFF authoring layer. A Go package that turns user-shaped input
into a `Blueprint` (the structured shape of a KIFF domain) and renders
that blueprint into convention-faithful artifacts: `kiff.yaml`, a Go
constants block, executor stubs, a test scaffold, and a full scaffold
that mirrors `kiff new -template=agentic-ops`.

MIT-licensed. Sibling to [`github.com/kiffhq/kiff`](https://github.com/kiffhq/kiff).

## Why a package, not a web product

Studio's value lives in the package, not in any specific surface. See
[RFC 012](https://github.com/kiffhq/kiff-cloud/blob/main/docs/design/012-studio-mvp.md)
for the full design rationale. The short version:

- **Studio as a layer.** The authoring concern in the protocol's
  vocabulary — what a domain's YAML, constants, executors, and tests
  look like.
- **Studio as a surface.** A specific UI for a specific audience —
  Cloud's dashboard, the kiff CLI, an eventual `studio.kiff.dev`.

The layer is what's load-bearing. The surface varies by audience. A
package serves both: pure functions over data, rendered by whatever
consumer needs them.

## Three rules govern the package

1. **Studio produces; it does not host.** The package returns bytes
   and structured values. It does not call into any KIFF runtime.
2. **Studio's output is convention-faithful.** Every artifact parses
   against the framework's domain validator and follows the
   framework's conventions for naming and structure.
3. **Studio depends on the framework, not on the cloud.** The
   dependency is one-way; `kiff-studio` knows nothing about
   `kiff-cloud`.

## Quick start

```go
import "github.com/kiffhq/kiff-studio"

input := studio.BuilderInput{
    WorkflowName: "refund-flow",
    EntityType:   "Order",
    States:       []string{"READY_FOR_REFUND", "REFUNDED"},
    Actions: []studio.ActionInput{{
        Name:               "ISSUE_REFUND",
        AllowedStates:      []string{"READY_FOR_REFUND"},
        RequiredParameters: []string{"amount", "reason"},
        ApprovalRequired:   true,
    }},
}

bp, err := studio.GenerateBlueprint(ctx, input)
if err != nil { /* handle */ }

yaml, _ := studio.BuildYAML(bp)
constants, _ := studio.BuildConstants(bp, "domain")
executors, _ := studio.BuildExecutorStub(bp, "domain")
tests, _ := studio.BuildTestScaffold(bp, "domain")

// Or render the full multi-file scaffold:
scaffold, _ := studio.BuildScaffold(bp, "github.com/acme/orders")
for path, body := range scaffold.Files {
    // write body to <root>/path
}
```

## The Go API (v0.1)

The primary entry point is `GenerateBlueprint`. It accepts four
input shapes:

| Input | Path | Notes |
|---|---|---|
| `BuilderInput` | deterministic | structured form, no LLM |
| `DescriptionInput` | LLM-assisted | plain-English → blueprint |
| `URLInput` | LLM-assisted | URL → blueprint |
| `ExistingCodeInput` | LLM-assisted | code snippet → blueprint |

The deterministic path has zero non-stdlib non-framework dependencies.
The LLM-assisted shapes require an `LLMGenerator` wired through
`WithLLMGenerator(...)`; v0.1 ships the interface only — the
consumer (Cloud's wrapper, framework-only adopters) provides the
concrete implementation. Tests use a stub.

## The artifact builders

All builders take a `Blueprint` and return `[]byte`:

- `BuildYAML(bp)` — `kiff.yaml` the framework's domain validator
  accepts on the first try.
- `BuildConstants(bp, packageName)` — Go constants block per the
  framework's conventions.
- `BuildExecutorStub(bp, packageName)` — Go domain file with
  contract factories, executor stubs, `NewRuntime`, `NewDefinition`,
  `NewPermissionPolicy`, `NewInputAdapter`.
- `BuildTestScaffold(bp, packageName)` — Go test file with the three
  convention-required cases (happy path, blocked path, replay).
- `BuildScaffold(bp, modulePath)` — multi-file `Scaffold` mirroring
  the agentic-ops template tree.

Every builder validates the `Blueprint` first; an invalid blueprint
surfaces as the validation error rather than producing partial output.

## Conformance posture

The package's `conformance_test.go` walks the framework's
`kiff new -template=agentic-ops` template tree and asserts the
scaffold output covers the same load-bearing files (the Go domain
layer + cmd/server entry point + kiff.yaml) and emits the same
wiring functions (`NewRuntime`, `NewDefinition`, `NewPermissionPolicy`,
`NewInputAdapter`, `Contracts`).

The test is a drift detector at v0.1: it asserts shape parity, not
byte-for-byte equality, because the agentic-ops template is
hand-tuned and `kiff-studio` emits a generic stub. Post-MVP step 5
of [RFC 012](https://github.com/kiffhq/kiff-cloud/blob/main/docs/design/012-studio-mvp.md),
the kiff CLI starts importing `kiff-studio` for its own scaffold
rendering — at that point both sides come from the same package
and the test tightens to byte-for-byte.

## Dependencies

The package itself has **zero non-stdlib runtime dependencies**.
Generated artifacts (the YAML, the executor stub, the test
scaffold) reference the framework's packages at the consumer's
project level. The package itself stays import-free of the
framework so that consumers control the framework version they
pin.

The LLM-assisted path's external dependency (a model client)
lives in the consumer, not the package — see "The Go API"
above.

## Status

v0.1.0 — first release.

See [RFC 012 (with 2026-05-28 amendment)](https://github.com/kiffhq/kiff-cloud/blob/main/docs/design/012-studio-mvp.md)
for the design.
