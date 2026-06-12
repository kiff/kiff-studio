// Package studio is the KIFF authoring layer.
//
// See README.md for an overview. The full design is in RFC 012:
// https://github.com/kiff/kiff-cloud/blob/main/docs/design/012-studio-mvp.md
//
// The package's primary entry point is GenerateBlueprint, which
// validates the caller's Input, dispatches it to the right
// generator (deterministic for BuilderInput, LLM-assisted for the
// description / URL / existing-code shapes), validates the
// resulting Blueprint, and returns it. Consumers then call
// BuildYAML, BuildConstants, BuildExecutorStub, BuildTestScaffold,
// or BuildScaffold to render artifacts from that Blueprint.
//
// Three rules govern the package:
//
//  1. Studio produces; it does not host. No runtime calls.
//  2. Studio's output is convention-faithful. Every artifact
//     parses against the framework's domain validator and follows
//     the framework's conventions for naming and structure.
//  3. Studio depends on the framework, not on the cloud. One-way
//     dependency.
//
// The package is MIT-licensed and ships independently of the
// cloud's release cycle.
package studio
