/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

// Package buildvars centralizes the environment variables that configure
// the dgraph build: binary name, Docker image tags, build-toolchain refs,
// and derived paths such as /gobin/<binary>. It replaces ad-hoc os.Getenv
// calls scattered across the codebase with a single enumerated, typed
// registry.
//
// Each Var carries its env-var name and a defaulter: either a literal
// string or a function that computes the default at Get() time. The
// function form supports Vars whose default depends on other Vars, such
// as /gobin/<BinaryName>.
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
// The package depends on no private-fork tree. A vendor or downstream
// that removes all fork-specific files sees only the upstream defaults
// declared here.
package buildvars

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// shellOutput runs cmd with args, captures stdout, and returns it stripped
// of trailing whitespace. Returns empty on error: for example, when the
// command is missing (git absent) or the working directory lacks a git
// repo. Used by the derived Vars whose defaults are git metadata.
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
// forms are exclusive; one is always nil.
//
// Instances are pointers. Call sites reference exported constants such as
// [BinaryName] and resolve values via [Var.Get].
//
// Literal defaults are mutable via [Var.SetDefault] so forks can override
// from their own init() without affecting call sites. SetDefault must run
// during package init() before any goroutines exist; the type performs no
// synchronization because no concurrent reader/writer scenario is
// reachable in practice.
type Var struct {
	// Name is the literal environment variable name. Set at declaration
	// and treated as immutable; do not mutate after package init.
	Name string

	// defaultValue is the registered literal fall-through when no env
	// override is set. Mutually exclusive with defaulter: when
	// defaulter is non-nil, it computes the default and defaultValue
	// is empty; when defaulter is nil, defaultValue holds the answer
	// (possibly the empty string, which is a valid default for vars
	// like ComposeBuildDir).
	defaultValue string

	// defaulter, when non-nil, computes the default at Get() time.
	// Used for derived Vars whose fall-through depends on other Vars,
	// such as a path built from another Var's current value.
	// [Var.SetDefault] freezes the result by clearing defaulter and
	// writing the literal to defaultValue.
	defaulter func() string
}

// newVar constructs a Var with a literal default. Used by the package's
// own declarations; unexported because call sites must not create new
// Vars outside this package.
func newVar(name, initialDefault string) *Var {
	return &Var{Name: name, defaultValue: initialDefault}
}

// newDerivedVar constructs a Var whose default is computed at Get() time
// by calling the supplied function. Useful for Vars whose fall-through
// depends on other Vars, such as GoBinDgraphPath = "/gobin/" + BinaryName.Get().
// The function runs on every Get() that falls through and should be
// cheap. [Var.SetDefault] can still replace the computation with a
// literal value.
func newDerivedVar(name string, defaulter func() string) *Var {
	return &Var{Name: name, defaulter: defaulter}
}

// Get returns the value of the environment variable when set and
// non-empty; otherwise the currently registered or computed default.
// Call sites prefer Get to os.Getenv so defaults can be overridden
// centrally without changing the call site.
func (v *Var) Get() string {
	if val := os.Getenv(v.Name); val != "" {
		return val
	}
	if v.defaultValue != "" || v.defaulter == nil {
		return v.defaultValue
	}
	return v.defaulter()
}

// Default returns the currently registered default and ignores any env
// override. For derived Vars it triggers the computation. Exposed for
// tests that mutate via [Var.SetDefault] and must capture the prior
// value to restore later, and for diagnostic tooling that logs the
// default independent of any env override.
func (v *Var) Default() string {
	if v.defaultValue != "" || v.defaulter == nil {
		return v.defaultValue
	}
	return v.defaulter()
}

// SetDefault replaces the registered default value with a literal string.
// For derived Vars it also clears the defaulter function, freezing the
// default at the new literal. Run once during package init() before any
// goroutines exist; the type performs no synchronization.
func (v *Var) SetDefault(value string) {
	v.defaultValue = value
	v.defaulter = nil
}

// Export sets the process-level environment variable named by v.Name to
// the Var's currently resolved value, as returned by [Var.Get]. Useful
// when spawning subprocesses that read their own os.Getenv: call
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
// points that must materialize the entire buildvars registry into the
// process environment before spawning subprocesses.
func ExportAll() error {
	for _, v := range All {
		if err := v.Export(); err != nil {
			return err
		}
	}
	return nil
}

// The canonical set of build-configuration vars. Each constant
// initializes with the upstream OSS default value. Forks override via
// the SetDefault method from their own init().
var (
	// BinaryName is the name of the dgraph binary at build time (what
	// `go build -o` writes, matching upstream's $(BIN) in
	// dgraph/Makefile) and at runtime (what compose files and test
	// harnesses invoke as /gobin/$BIN). Upstream default: "dgraph".
	// Forks rename via env or SetDefault, e.g. "myfork-dgraph".
	//
	// The env-var name remains BIN for backward compatibility with
	// shell scripts, Makefiles, and CI configs that reference $(BIN)
	// or ${BIN} directly. The Go identifier is BinaryName because Go
	// callers benefit from the more descriptive symbol.
	BinaryName = newVar("BIN", "dgraph")

	// DockerImage is the Docker image tag (without :tag suffix) used by
	// the Makefile local-image and docker-image targets and by the
	// compose generator's --image default. Upstream default:
	// "dgraph/dgraph".
	DockerImage = newVar("DOCKER_IMAGE", "dgraph/dgraph")

	// BuildImage is the toolchain image for the Dockerfile build stage.
	// Upstream uses an OS base image; forks may substitute a hardened
	// toolchain such as Chainguard go-fips. Paired with BuildTag.
	BuildImage = newVar("BUILD_IMAGE", "ubuntu")

	// BuildTag is the tag for BuildImage. Upstream default: "24.04".
	BuildTag = newVar("BUILD_TAG", "24.04")

	// RuntimeImage is the runtime base image for the Dockerfile runtime
	// stage. Paired with RuntimeTag.
	RuntimeImage = newVar("RUNTIME_IMAGE", "ubuntu")

	// RuntimeTag is the tag for RuntimeImage.
	RuntimeTag = newVar("RUNTIME_TAG", "24.04")

	// ComposeBuildDir holds generated docker-compose overlays produced
	// by a compose-rewriting build step. Test harnesses route
	// docker-compose invocations through this directory when set.
	// Upstream default: empty (no overlay).
	ComposeBuildDir = newVar("DGRAPH_COMPOSE_BUILD_DIR", "")

	// BuildTags is the space-separated list of Go build tags passed to
	// `go build` via -tags. The upstream default composes "jemalloc"
	// (the well-known libjemalloc tag) with any tags a fork adds via
	// [CustomBuildTags]. The Makefile and this Var share the same
	// composition, so `make` and direct `go run` agree on the final
	// tag set.
	//
	// "jemalloc" is meaningful only on Linux and macOS; the Makefile
	// omits it on other hosts. Go code reading BuildTags treats the
	// value as informational and makes no OS-aware decisions. The
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
	// Consumers needing compile-time code paths set this to
	// space-separated tag names, e.g. "myfork fips". The Makefile
	// concatenates CustomBuildTags onto BuildTags, so callers read
	// BuildTags to see the final tag set.
	CustomBuildTags = newVar("CUSTOM_BUILD_TAGS", "")

	// GoRunTags is the build-tag list passed via -tags to `go run`
	// invocations from Makefile helper recipes. For example, `build-env`
	// runs `go run -tags=$(GO_RUN_TAGS) ./buildvars/cmd/buildvars` so
	// the buildvars CLI picks up a fork's init-time default overrides.
	//
	// Distinct from [BuildTags]: GoRunTags contains only the tags
	// needed to trigger fork-specific init()-time registration,
	// typically just a fork's primary tag. BuildTags adds "jemalloc"
	// and any other compile-time-only tags a long-running binary needs.
	//
	// Upstream default: empty. Forks override via an init()-time
	// SetDefault call in the package their registration tag guards.
	GoRunTags = newVar("GO_RUN_TAGS", "")

	// DgraphVersion is the version tag baked into the Docker image and
	// into the binary's version output. Upstream default: "local",
	// the development build tag. CI overrides via env to the release
	// tag on tagged builds, e.g. "v25.3.0".
	DgraphVersion = newVar("DGRAPH_VERSION", "local")

	// Build is the short git commit SHA of the build source, baked
	// into the binary's version output via -ldflags -X. The default
	// computes at Get() time from `git rev-parse --short HEAD` and is
	// empty when the build happens outside a git working copy.
	Build = newDerivedVar("BUILD", func() string {
		return shellOutput("git", "rev-parse", "--short", "HEAD")
	})

	// BuildDate is the commit timestamp of the build source, baked
	// into the binary via -ldflags -X. The default computes from
	// `git log -1 --format=%ci`.
	BuildDate = newDerivedVar("BUILD_DATE", func() string {
		return shellOutput("git", "log", "-1", "--format=%ci")
	})

	// BuildBranch is the git branch of the build source, baked in via
	// -ldflags -X. The default computes from
	// `git rev-parse --abbrev-ref HEAD`.
	BuildBranch = newDerivedVar("BUILD_BRANCH", func() string {
		return shellOutput("git", "rev-parse", "--abbrev-ref", "HEAD")
	})

	// BuildCodename is a human-readable name for the release family,
	// baked into the binary via -ldflags -X. Upstream default: "dgraph";
	// CI overrides per release.
	BuildCodename = newVar("BUILD_CODENAME", "dgraph")

	// BuildVersion is the full version string baked into the binary,
	// e.g. "v25.3.0-5-gabc1234". The default computes from
	// `git describe --always --tags` and is empty when git is
	// unavailable. CI release builds override via env to the exact
	// release tag.
	BuildVersion = newDerivedVar("BUILD_VERSION", func() string {
		return shellOutput("git", "describe", "--always", "--tags")
	})

	// LinuxGobin is the host directory holding the Linux-built dgraph
	// binary used for bind-mounting into containers. On Linux, usually
	// $GOPATH/bin (native); on macOS, a separate $GOPATH/linux_<arch>/
	// directory so the cross-compiled Linux binary does not collide
	// with the Mach-O host binary. The default computes at Get() time
	// from $GOPATH + runtime.GOARCH.
	LinuxGobin = newDerivedVar("LINUX_GOBIN", func() string {
		gopath := os.Getenv("GOPATH")
		if gopath == "" {
			return ""
		}
		return gopath + "/linux_" + runtime.GOARCH
	})

	// GoBinDgraphPath is the in-container path to the dgraph binary:
	// the bind-mounted /gobin directory joined with the binary name.
	// Used by jepsen's --local-binary default and by the compose
	// generator for services configured with LocalBin. Derived from
	// [BinaryName] at Get() time so the path tracks the binary name.
	GoBinDgraphPath = newDerivedVar("GOBIN_DGRAPH_PATH", func() string {
		return "/gobin/" + BinaryName.Get()
	})

	// HostGopathDgraphPath is the host-side absolute path to the built
	// dgraph binary, $GOPATH/bin/<BinaryName>. Used by testutil and
	// t/t.go to locate the binary on the runner host outside
	// containers. Returns empty when $GOPATH is unset so callers can
	// fall back to build.Default.GOPATH or their own resolution.
	// Derived from [BinaryName] at Get() time.
	HostGopathDgraphPath = newDerivedVar("HOST_GOPATH_DGRAPH_PATH", func() string {
		gopath := os.Getenv("GOPATH")
		if gopath == "" {
			return ""
		}
		return gopath + "/bin/" + BinaryName.Get()
	})
)

// All lists every defined Var in declaration order. Exposed so tooling
// such as diagnostic dumps, documentation generators, and resolver
// overrides can iterate the canonical set.
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
