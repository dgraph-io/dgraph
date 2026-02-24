# Repository Guidelines

## Project Structure & Module Organization

The Go module is rooted here, with entrypoints and CLI tooling under `dgraph/` for Alpha, Zero,
Live, and companions. Core execution sits in `worker/`, `query/`, and `posting/`, the GraphQL
adapters in `graphql/`, and shared helpers in packages like `x/` and `testutil/`. Protocol
definitions live in `protos/` (regenerate via `make regenerate` inside that folder). Integration
harnesses are in `t/`, broader system scenarios in `systest/`, and container assets under `contrib/`
and `compose/`.

## Build, Test, and Development Commands

Use `make dgraph` to compile the Linux AMD64 binaries into `./dgraph/dgraph`, or `make install` to
drop them into `$GOPATH/bin`. Run `go test ./path/to/package` for focused checks or `go test ./...`
for a fast repo-wide sweep. `make test` (no args) runs the default suite (`unit,systest,core` +
`integration2`, ~30 min); use `make test SUITE=all` for all t/ runner suites or `make test-full` for
every test in the repo. Keep Docker running for integration tests. When debugging version info,
`make version` surfaces the embedded build metadata.

## Coding Style & Naming Conventions

Follow standard Go conventions: exported identifiers start with capitals, packages use lowercase,
and tests use `TestXxx` names. Format every change with `go fmt` (or `gofmt -w`) before committing;
CI runs `golangci-lint`, so keep `//nolint` waivers rare and justified. Maintain the SPDX license
header block on all new source files and avoid introducing unused helpers or dead code.

## Testing Guidelines

Unit tests live beside their packages in `_test.go` files and run with `go test`. The `t/` harness
drives integration clusters; add new scenarios under targeted subdirectories. `systest/` and
`graphql/e2e/` capture longer system flows—mirror their layout when extending coverage. Before
opening a PR, run `go test ./...` and the `t` or `systest` suites touched by your change, and
include regression tests for new behaviour.

## Commit & Pull Request Guidelines

Recent history favors `type(scope): short message` commits (for example, `fix(compose): ...` or
`chore: ...`). Keep commits atomic with clear intent, sign when possible, and reference issues using
`Fixes #1234` when applicable. PRs should describe the motivation, summarize testing
(`go test ./...`, `make test`, etc.), and call out configuration or data migrations. Include doc
updates for user-facing behaviour changes and attach CLI output or screenshots if tooling UX shifts.
