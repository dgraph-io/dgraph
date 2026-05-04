/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

// Package buildvars centralizes the environment variables that configure
// the dgraph build (binary name, Docker image tags, build-toolchain refs,
// derived paths like /gobin/<binary>, etc.). It exists to replace ad-hoc
// os.Getenv calls with literal keys scattered across the codebase with a
// single enumerated, typed registry.
//
// Each Var carries its own env-var name and a defaulter — either a literal
// string or a function that computes the default at Get() time (useful for
// Vars whose default depends on other Vars, e.g. /gobin/<BinaryName>).
//
// Usage:
//
//	bin := buildvars.BinaryName.Get()       // reads $BIN or default
//	path := buildvars.GoBinDgraphPath.Get() // "/gobin/<BinaryName value>"
//
// Forks override literal defaults from their own package init():
//
//	func init() {
//	    buildvars.BinaryName.SetDefault("myfork-dgraph")
//	}
//
// Derived Vars automatically pick up the override because they recompute
// at Get() time.
//
// The package has no dependency on any private-fork tree; a vendor or
// downstream that removes all fork-specific files sees only the
// upstream defaults declared here.
package buildvars

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
)

// shellOutput runs cmd with args, captures stdout, and returns it stripped
// of trailing whitespace. Returns empty on error — e.g. if the command is
// not available (git absent) or the working directory is not a git repo.
// Used by the derived Vars whose defaults are git metadata.
func shellOutput(cmd string, args ...string) string {
	out, err := exec.Command(cmd, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Var is a single build-configuration environment variable. It bundles
// the env-var name (Name) with a defaulter: either a literal fall-through
// string or a function that computes the default at Get() time. The two
// forms are exclusive — one is always nil.
//
// Instances are pointers; call sites reference exported constants (e.g.
// [BinaryName]) and resolve values via the [Var.Get] method.
//
// Literal defaults are mutable via [Var.SetDefault] so forks can override
// from their own init() without affecting call sites. The mutation is
// guarded by an internal mutex; all reads are consistent.
type Var struct {
	// Name is the literal environment variable name. Set at declaration
	// and treated as immutable; do not mutate after package init.
	Name string

	// defaultValue is the registered fall-through when this Var has a
	// literal (non-computed) default. Guarded by mu. Used iff defaulter
	// is nil.
	defaultValue string

	// defaulter computes the default at Get() time. Used for derived
	// Vars whose fall-through depends on other Vars (e.g. a path built
	// from another Var's current value). When non-nil, [Var.SetDefault]
	// still works but overrides the computation with a literal.
	defaulter func() string

	mu sync.RWMutex
}

// newVar constructs a Var with a literal default. Used by the package's
// own declarations; not exported because call sites should not create
// new Vars outside of this package.
func newVar(name, initialDefault string) *Var {
	return &Var{Name: name, defaultValue: initialDefault}
}

// newDerivedVar constructs a Var whose default is computed at Get() time
// by calling the supplied function. Useful for Vars whose fall-through
// depends on other Vars (e.g. GoBinDgraphPath = "/gobin/" + BinaryName.Get()).
// The function is called on every Get() that falls through; it should be
// cheap. [Var.SetDefault] can still be called to replace the computation
// with a literal value.
func newDerivedVar(name string, defaulter func() string) *Var {
	return &Var{Name: name, defaulter: defaulter}
}

// Get returns the value of the environment variable if set and non-empty,
// otherwise the currently-registered or computed default. Call sites use
// this in preference to os.Getenv so defaults can be overridden centrally
// without changing the call site.
func (v *Var) Get() string {
	if val := os.Getenv(v.Name); val != "" {
		return val
	}
	v.mu.RLock()
	literal := v.defaultValue
	literalSet := literal != "" || v.defaulter == nil
	defaulter := v.defaulter
	v.mu.RUnlock()
	if literalSet {
		return literal
	}
	return defaulter()
}

// Default returns the currently-registered default (ignoring any env
// override). For derived Vars this triggers the computation. Exposed
// primarily for diagnostic tooling that needs to log what the default
// would be (independent of what any env override would produce).
func (v *Var) Default() string {
	v.mu.RLock()
	literal := v.defaultValue
	literalSet := literal != "" || v.defaulter == nil
	defaulter := v.defaulter
	v.mu.RUnlock()
	if literalSet {
		return literal
	}
	return defaulter()
}

// SetDefault replaces the registered default value with a literal string.
// For derived Vars this also clears the defaulter function, freezing the
// default at the new literal. Typically called from a fork's package
// init() to override upstream defaults with fork-specific values. Safe
// to call concurrently but intended to run once at startup before any
// Get calls.
func (v *Var) SetDefault(value string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.defaultValue = value
	v.defaulter = nil
}

// Export sets the process-level environment variable named by v.Name to
// the Var's currently-resolved value (as returned by [Var.Get]). Useful
// when spawning subprocesses that read their own os.Getenv — call
// v.Export() before exec to make the registered default visible to the
// child as an explicit env entry.
//
// Idempotent: when the env var is already set, Get returns the existing
// value and Export writes the same value back. Safe to call repeatedly.
//
// Returns any error from os.Setenv (rare).
func (v *Var) Export() error {
	return os.Setenv(v.Name, v.Get())
}

// ExportAll calls [Var.Export] on every Var in [All] and returns the
// first error encountered, or nil on success. Convenient for entry
// points that need the entire buildvars registry materialized into the
// process environment before spawning subprocesses.
func ExportAll() error {
	for _, v := range All {
		if err := v.Export(); err != nil {
			return err
		}
	}
	return nil
}

// The canonical set of build-configuration vars. Each constant is
// initialized with the upstream OSS default value. Forks override via
// the SetDefault method from their own init().
var (
	// BinaryName is the name of the dgraph binary — both at build time
	// (what `go build -o` writes, matching upstream's $(BIN) in
	// dgraph/Makefile) and at runtime (what compose files and test
	// harnesses invoke as /gobin/$BIN). Upstream default: "dgraph".
	// Forks rename (e.g. "myfork-dgraph") via env or SetDefault.
	//
	// The env-var name is kept as BIN for backward compatibility with
	// shell scripts, Makefiles, and CI configs that reference $(BIN) /
	// ${BIN} directly. The Go identifier is BinaryName because Go
	// callers benefit from the more descriptive symbol.
	BinaryName = newVar("BIN", "dgraph")

	// DockerImage is the Docker image tag (without :tag suffix) used by
	// Makefile local-image / docker-image targets and by the compose
	// generator's --image default. Upstream default: "dgraph/dgraph".
	DockerImage = newVar("DOCKER_IMAGE", "dgraph/dgraph")

	// BuildImage is the toolchain image used as the Dockerfile build
	// stage. Upstream uses an OS base image; forks may substitute a
	// hardened toolchain (e.g. Chainguard go-fips). Paired with BuildTag.
	BuildImage = newVar("BUILD_IMAGE", "ubuntu")

	// BuildTag is the tag for BuildImage. Upstream default: "24.04".
	BuildTag = newVar("BUILD_TAG", "24.04")

	// RuntimeImage is the runtime base image for the Dockerfile runtime
	// stage. Paired with RuntimeTag.
	RuntimeImage = newVar("RUNTIME_IMAGE", "ubuntu")

	// RuntimeTag is the tag for RuntimeImage.
	RuntimeTag = newVar("RUNTIME_TAG", "24.04")

	// ComposeBuildDir is the directory that holds generated docker-compose
	// overlays produced by a compose-rewriting build step. Test harnesses
	// route docker-compose invocations through this directory when set.
	// Upstream default: empty (no overlay).
	ComposeBuildDir = newVar("DGRAPH_COMPOSE_BUILD_DIR", "")

	// BuildTags is the space-separated list of Go build tags passed to
	// `go build` (via -tags). Upstream default composes "jemalloc" (the
	// well-known libjemalloc tag) with any tags a fork adds via
	// [CustomBuildTags]. Both the Makefile and this Var use the same
	// composition, so `make` and direct `go run` agree on the final
	// tag set.
	//
	// Note: "jemalloc" is only meaningful on Linux/macOS (the Makefile
	// omits it on other hosts). Go code reading BuildTags doesn't make
	// OS-aware decisions with the value; it's informational. The
	// Makefile handles OS gating for the actual `go build` invocation.
	BuildTags = newDerivedVar("BUILD_TAGS", func() string {
		base := "jemalloc"
		extra := CustomBuildTags.Get()
		if extra == "" {
			return base
		}
		return base + " " + extra
	})

	// CustomBuildTags is the list of additional Go build tags a fork or
	// downstream consumer appends to [BuildTags]. Upstream default: empty.
	// Consumers that need compile-time code paths set this to space-
	// separated tag names (e.g. "myfork fips"). The Makefile
	// concatenates CustomBuildTags onto BuildTags, so callers typically
	// read BuildTags to see the final tag set.
	CustomBuildTags = newVar("CUSTOM_BUILD_TAGS", "")

	// GoRunTags is the build-tag list passed via -tags to `go run`
	// invocations from Makefile helper recipes (e.g. `build-env` runs
	// `go run -tags=$(GO_RUN_TAGS) ./buildvars/cmd/buildvars` so the
	// buildvars CLI picks up a fork's init-time default overrides).
	//
	// Distinct from [BuildTags]: GoRunTags contains only the tags
	// needed to trigger fork-specific init()-time registration
	// (typically just a fork's primary tag). BuildTags adds "jemalloc"
	// and any other compile-time-only tags a long-running binary needs.
	//
	// Upstream default: empty. Forks override via an init()-time
	// SetDefault call in the package their registration tag guards.
	GoRunTags = newVar("GO_RUN_TAGS", "")

	// DgraphVersion is the version tag baked into the Docker image and
	// into the binary's version output. Upstream default: "local" (the
	// development build tag). CI overrides via env to the release tag
	// on tagged builds (e.g. "v25.3.0").
	DgraphVersion = newVar("DGRAPH_VERSION", "local")

	// Build is the short git commit SHA of the build source. Baked into
	// the binary's version output via -ldflags -X. Default is computed
	// at Get() time from `git rev-parse --short HEAD` — empty if the
	// build happens outside a git working copy.
	Build = newDerivedVar("BUILD", func() string {
		return shellOutput("git", "rev-parse", "--short", "HEAD")
	})

	// BuildDate is the commit timestamp of the build source. Default
	// computed from `git log -1 --format=%ci`. Baked into the binary
	// via -ldflags -X.
	BuildDate = newDerivedVar("BUILD_DATE", func() string {
		return shellOutput("git", "log", "-1", "--format=%ci")
	})

	// BuildBranch is the git branch the build source was on. Default
	// computed from `git rev-parse --abbrev-ref HEAD`. Baked in via
	// -ldflags -X.
	BuildBranch = newDerivedVar("BUILD_BRANCH", func() string {
		return shellOutput("git", "rev-parse", "--abbrev-ref", "HEAD")
	})

	// BuildCodename is a human-readable name for the release family
	// (e.g. "dgraph" upstream, overridden per-release by CI). Baked
	// into the binary via -ldflags -X.
	BuildCodename = newVar("BUILD_CODENAME", "dgraph")

	// BuildVersion is the full version string baked into the binary
	// (e.g. "v25.3.0-5-gabc1234"). Default computed from
	// `git describe --always --tags` — empty if git isn't available.
	// CI release builds override via env to the exact release tag.
	BuildVersion = newDerivedVar("BUILD_VERSION", func() string {
		return shellOutput("git", "describe", "--always", "--tags")
	})

	// LinuxGobin is the host directory holding the Linux-built dgraph
	// binary used for bind-mounting into containers. On Linux, usually
	// $GOPATH/bin (native); on macOS, a separate $GOPATH/linux_<arch>/
	// directory so the cross-compiled Linux binary doesn't collide with
	// the Mach-O host binary. Default computed at Get() time from
	// $GOPATH + runtime.GOARCH.
	LinuxGobin = newDerivedVar("LINUX_GOBIN", func() string {
		gopath := os.Getenv("GOPATH")
		if gopath == "" {
			return ""
		}
		return gopath + "/linux_" + runtime.GOARCH
	})

	// GoBinDgraphPath is the in-container path to the dgraph binary (i.e.
	// the bind-mounted /gobin directory + the binary name). Used by
	// jepsen's --local-binary default and by the compose generator for
	// services configured with LocalBin. Derived from [BinaryName] at
	// Get() time so the path automatically tracks the binary name.
	GoBinDgraphPath = newDerivedVar("GOBIN_DGRAPH_PATH", func() string {
		return "/gobin/" + BinaryName.Get()
	})

	// HostGopathDgraphPath is the host-side absolute path to the built
	// dgraph binary ($GOPATH/bin/<BinaryName>). Used by testutil and t/t.go
	// to locate the binary on the runner host (outside of containers).
	// Returns empty if $GOPATH is unset so callers can fall back to
	// build.Default.GOPATH or their own resolution. Derived from
	// [BinaryName] at Get() time.
	HostGopathDgraphPath = newDerivedVar("HOST_GOPATH_DGRAPH_PATH", func() string {
		gopath := os.Getenv("GOPATH")
		if gopath == "" {
			return ""
		}
		return gopath + "/bin/" + BinaryName.Get()
	})
)

// All lists every defined Var in declaration order. Exposed so that
// tooling (diagnostic dumps, documentation generators, resolver overrides)
// can iterate the canonical set.
var All = []*Var{
	BinaryName,
	DockerImage,
	BuildImage,
	BuildTag,
	RuntimeImage,
	RuntimeTag,
	ComposeBuildDir,
	BuildTags,
	CustomBuildTags,
	GoRunTags,
	DgraphVersion,
	Build,
	BuildDate,
	BuildBranch,
	BuildCodename,
	BuildVersion,
	LinuxGobin,
	GoBinDgraphPath,
	HostGopathDgraphPath,
}
