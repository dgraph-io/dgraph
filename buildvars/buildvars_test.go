/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package buildvars

import (
	"os"
	"testing"
)

func TestGet_EnvSet(t *testing.T) {
	v := newVar("TEST_GET_ENV_SET", "literal-default")
	t.Setenv("TEST_GET_ENV_SET", "from-env")
	if got := v.Get(); got != "from-env" {
		t.Errorf("Get() = %q, want %q", got, "from-env")
	}
}

func TestGet_EnvUnset_UsesLiteral(t *testing.T) {
	v := newVar("TEST_GET_ENV_UNSET", "literal-default")
	// Ensure env is not set
	os.Unsetenv("TEST_GET_ENV_UNSET")
	if got := v.Get(); got != "literal-default" {
		t.Errorf("Get() = %q, want %q", got, "literal-default")
	}
}

func TestGet_EmptyEnvUsesLiteral(t *testing.T) {
	v := newVar("TEST_GET_EMPTY", "literal-default")
	t.Setenv("TEST_GET_EMPTY", "")
	if got := v.Get(); got != "literal-default" {
		t.Errorf("Get() = %q with empty env, want %q", got, "literal-default")
	}
}

func TestSetDefault_OverridesLiteral(t *testing.T) {
	v := newVar("TEST_SET_DEFAULT", "initial")
	os.Unsetenv("TEST_SET_DEFAULT")
	v.SetDefault("overridden")
	if got := v.Get(); got != "overridden" {
		t.Errorf("Get() after SetDefault = %q, want %q", got, "overridden")
	}
}

func TestSetDefault_EnvStillWins(t *testing.T) {
	v := newVar("TEST_SET_DEFAULT_ENV", "initial")
	v.SetDefault("overridden")
	t.Setenv("TEST_SET_DEFAULT_ENV", "from-env")
	if got := v.Get(); got != "from-env" {
		t.Errorf("Get() with env set = %q, want env value %q", got, "from-env")
	}
}

func TestDerivedVar_UsesDefaulter(t *testing.T) {
	base := newVar("TEST_DERIVED_BASE", "base-value")
	os.Unsetenv("TEST_DERIVED_BASE")
	derived := newDerivedVar("TEST_DERIVED", func() string {
		return "/prefix/" + base.Get()
	})
	os.Unsetenv("TEST_DERIVED")
	if got := derived.Get(); got != "/prefix/base-value" {
		t.Errorf("Derived Get() = %q, want %q", got, "/prefix/base-value")
	}
}

func TestDerivedVar_TracksBaseOverride(t *testing.T) {
	base := newVar("TEST_DERIVED_TRACK_BASE", "initial")
	os.Unsetenv("TEST_DERIVED_TRACK_BASE")
	derived := newDerivedVar("TEST_DERIVED_TRACK", func() string {
		return "/prefix/" + base.Get()
	})
	os.Unsetenv("TEST_DERIVED_TRACK")

	// Before override
	if got := derived.Get(); got != "/prefix/initial" {
		t.Fatalf("pre-override: Get() = %q, want %q", got, "/prefix/initial")
	}

	// Override the base; derived should track
	base.SetDefault("overridden")
	if got := derived.Get(); got != "/prefix/overridden" {
		t.Errorf("post-override: Get() = %q, want %q", got, "/prefix/overridden")
	}
}

func TestDerivedVar_EnvOverride(t *testing.T) {
	base := newVar("TEST_DERIVED_ENV_BASE", "ignored-when-derived-env-set")
	derived := newDerivedVar("TEST_DERIVED_ENV", func() string {
		return "/computed/" + base.Get()
	})
	t.Setenv("TEST_DERIVED_ENV", "literal-env-value")
	if got := derived.Get(); got != "literal-env-value" {
		t.Errorf("derived with env set: Get() = %q, want %q", got, "literal-env-value")
	}
}

func TestDerivedVar_SetDefaultFreezes(t *testing.T) {
	base := newVar("TEST_DERIVED_FREEZE_BASE", "base-v1")
	os.Unsetenv("TEST_DERIVED_FREEZE_BASE")
	derived := newDerivedVar("TEST_DERIVED_FREEZE", func() string {
		return "/prefix/" + base.Get()
	})
	os.Unsetenv("TEST_DERIVED_FREEZE")

	// Freeze with an explicit literal
	derived.SetDefault("frozen-value")

	// Changing base should no longer affect derived
	base.SetDefault("base-v2")
	if got := derived.Get(); got != "frozen-value" {
		t.Errorf("after freeze: Get() = %q, want %q", got, "frozen-value")
	}
}

func TestDefault_LiteralVar(t *testing.T) {
	v := newVar("TEST_DEFAULT_LITERAL", "the-default")
	// Default() ignores env
	t.Setenv("TEST_DEFAULT_LITERAL", "env-value")
	if got := v.Default(); got != "the-default" {
		t.Errorf("Default() = %q, want %q (env override should be ignored)", got, "the-default")
	}
}

func TestDefault_DerivedVar(t *testing.T) {
	base := newVar("TEST_DEFAULT_DERIVED_BASE", "base-default")
	os.Unsetenv("TEST_DEFAULT_DERIVED_BASE")
	derived := newDerivedVar("TEST_DEFAULT_DERIVED", func() string {
		return "/computed/" + base.Get()
	})
	t.Setenv("TEST_DEFAULT_DERIVED", "env-value-should-be-ignored")
	if got := derived.Default(); got != "/computed/base-default" {
		t.Errorf("Default() of derived = %q, want %q", got, "/computed/base-default")
	}
}

// TestPackageDefaults exercises the actual declared Vars to catch any
// drift between the declarations and the documented upstream defaults.
func TestPackageDefaults(t *testing.T) {
	// Clear env vars under our control so Get() returns the registered
	// default. GOPATH is the user's dev-environment setting and we don't
	// touch it; HostGopathDgraphPath is tested separately below.
	for _, v := range All {
		if v.Name == "GOPATH" {
			continue
		}
		os.Unsetenv(v.Name)
	}

	cases := []struct {
		v    *Var
		want string
	}{
		{Bin, "dgraph"},
		{DockerImage, "dgraph/dgraph"},
		{BuildImage, "ubuntu"},
		{BuildTag, "24.04"},
		{RuntimeImage, "ubuntu"},
		{RuntimeTag, "24.04"},
		{ComposeBuildDir, ""},
		{GoBinDgraphPath, "/gobin/dgraph"},
	}
	for _, c := range cases {
		if got := c.v.Get(); got != c.want {
			t.Errorf("%s.Get() = %q, want %q", c.v.Name, got, c.want)
		}
	}
}

// TestHostGopathDgraphPath_DependsOnGopath verifies the HostGopathDgraphPath
// derived Var composes GOPATH + Bin correctly, and returns empty
// when GOPATH is unset (so callers can fall back to build.Default.GOPATH).
func TestHostGopathDgraphPath_DependsOnGopath(t *testing.T) {
	origDefault := Bin.Default()
	t.Cleanup(func() { Bin.SetDefault(origDefault) })

	os.Unsetenv(string(Bin.Name))
	os.Unsetenv(string(HostGopathDgraphPath.Name))

	// With GOPATH set
	t.Setenv("GOPATH", "/tmp/fake-gopath")
	if got := HostGopathDgraphPath.Get(); got != "/tmp/fake-gopath/bin/dgraph" {
		t.Errorf("with GOPATH=/tmp/fake-gopath: Get() = %q, want %q",
			got, "/tmp/fake-gopath/bin/dgraph")
	}

	// With GOPATH unset → empty return (caller falls back)
	os.Unsetenv("GOPATH")
	if got := HostGopathDgraphPath.Get(); got != "" {
		t.Errorf("with GOPATH unset: Get() = %q, want empty", got)
	}
}

func TestExport_WritesToEnv(t *testing.T) {
	v := newVar("TEST_EXPORT", "exported-default")
	os.Unsetenv("TEST_EXPORT")
	if err := v.Export(); err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if got := os.Getenv("TEST_EXPORT"); got != "exported-default" {
		t.Errorf("post-Export: os.Getenv(%q) = %q, want %q",
			"TEST_EXPORT", got, "exported-default")
	}
}

func TestExport_DerivedVar(t *testing.T) {
	base := newVar("TEST_EXPORT_DERIVED_BASE", "base-value")
	os.Unsetenv("TEST_EXPORT_DERIVED_BASE")
	derived := newDerivedVar("TEST_EXPORT_DERIVED", func() string {
		return "/prefix/" + base.Get()
	})
	os.Unsetenv("TEST_EXPORT_DERIVED")

	if err := derived.Export(); err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if got := os.Getenv("TEST_EXPORT_DERIVED"); got != "/prefix/base-value" {
		t.Errorf("post-Export of derived: os.Getenv(%q) = %q, want %q",
			"TEST_EXPORT_DERIVED", got, "/prefix/base-value")
	}
}

func TestExport_Idempotent(t *testing.T) {
	v := newVar("TEST_EXPORT_IDEMPOTENT", "literal-default")
	t.Setenv("TEST_EXPORT_IDEMPOTENT", "preset-value")

	// Preset env wins for Get, and Export writes that same value back.
	if err := v.Export(); err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if got := os.Getenv("TEST_EXPORT_IDEMPOTENT"); got != "preset-value" {
		t.Errorf("post-Export with preset env: os.Getenv(%q) = %q, want %q",
			"TEST_EXPORT_IDEMPOTENT", got, "preset-value")
	}

	// Calling again should be safe and produce the same state.
	if err := v.Export(); err != nil {
		t.Fatalf("second Export() error = %v", err)
	}
	if got := os.Getenv("TEST_EXPORT_IDEMPOTENT"); got != "preset-value" {
		t.Errorf("second Export: os.Getenv(%q) = %q, want %q",
			"TEST_EXPORT_IDEMPOTENT", got, "preset-value")
	}
}

func TestBuildTags_ComposesWithExtraBuildTags(t *testing.T) {
	origExtra := ExtraBuildTags.Default()
	t.Cleanup(func() { ExtraBuildTags.SetDefault(origExtra) })

	os.Unsetenv("BUILD_TAGS")
	os.Unsetenv("PRIVATE_BUILD_TAGS")

	// Empty extra → just "jemalloc"
	ExtraBuildTags.SetDefault("")
	if got := BuildTags.Get(); got != "jemalloc" {
		t.Errorf("with empty extra: BuildTags.Get() = %q, want %q", got, "jemalloc")
	}

	// Extra set → composed
	ExtraBuildTags.SetDefault("istari requirefips")
	if got := BuildTags.Get(); got != "jemalloc istari requirefips" {
		t.Errorf("with extra: BuildTags.Get() = %q, want %q",
			got, "jemalloc istari requirefips")
	}

	// Env override wins (Makefile passes composed value directly)
	t.Setenv("BUILD_TAGS", "jemalloc custom")
	if got := BuildTags.Get(); got != "jemalloc custom" {
		t.Errorf("with env override: BuildTags.Get() = %q, want %q",
			got, "jemalloc custom")
	}
}

func TestExportAll(t *testing.T) {
	// Clear all env vars we can, then ExportAll should set them to their
	// registered defaults.
	for _, v := range All {
		if v.Name == "GOPATH" {
			continue
		}
		os.Unsetenv(v.Name)
	}

	if err := ExportAll(); err != nil {
		t.Fatalf("ExportAll() error = %v", err)
	}

	// Spot-check a few: each should now have its default-value in env
	for _, v := range []*Var{Bin, DockerImage, GoBinDgraphPath} {
		got := os.Getenv(v.Name)
		want := v.Default()
		if got != want {
			t.Errorf("post-ExportAll: os.Getenv(%q) = %q, want %q",
				v.Name, got, want)
		}
	}
}

// TestGoBinDgraphPath_TracksBin verifies the specific derivation
// that the user called out: when Bin changes (via env or override),
// GoBinDgraphPath reflects that change.
func TestGoBinDgraphPath_TracksBin(t *testing.T) {
	// Preserve original state; the rest of the file is exercising
	// these in parallel-incompatible ways already.
	origDefault := Bin.Default()
	t.Cleanup(func() { Bin.SetDefault(origDefault) })

	os.Unsetenv(string(Bin.Name))
	os.Unsetenv(string(GoBinDgraphPath.Name))

	// Override via env
	t.Setenv("BIN", "dgraph-sec")
	if got := GoBinDgraphPath.Get(); got != "/gobin/dgraph-sec" {
		t.Errorf("via env: GoBinDgraphPath.Get() = %q, want %q", got, "/gobin/dgraph-sec")
	}

	// Override via SetDefault
	os.Unsetenv("BIN")
	Bin.SetDefault("dgraph-alt")
	if got := GoBinDgraphPath.Get(); got != "/gobin/dgraph-alt" {
		t.Errorf("via SetDefault: GoBinDgraphPath.Get() = %q, want %q", got, "/gobin/dgraph-alt")
	}
}
