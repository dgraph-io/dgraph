/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package testutil

// This file declares public extensibility hooks for testutil. Each hook
// is a package-level function-var initialized to an unexported default
// that implements stock upstream behavior. A private fork or downstream
// consumer reassigns the public var from its own init() to customize
// behavior without touching upstream code.
//
// Call sites such as StartAlphas in bulk.go invoke the hooks as ordinary
// package-level functions. Upstream builds run the default
// implementations; forks run whatever they installed.
//
// The defaults are deliberately unexported so external packages must
// reach the behavior through the public var: the reassignment channel.
// Tests and fork registrations save and restore the public var as
// needed.

// ComposeArgs returns the docker-compose file selector args:
// "-f <path>" plus any optional overlays such as --project-directory.
// Default: returns "-f <path>" unchanged. Forks may rewrite the path
// through a generator overlay and set --project-directory so relative
// bind-mount sources resolve against the caller's package dir.
var ComposeArgs = defaultComposeArgs

// EnvForCompose returns extra KEY=VALUE entries to inject into the
// environment of docker-compose subprocesses. Default: nil. Forks
// inject UID/GID so ${UID:-65532} in generated compose files resolves
// to the host UID rather than the image's nonroot user.
var EnvForCompose = defaultEnvForCompose

// BuildPlugins compiles the custom-tokenizer Go plugins used by
// systest/plugin tests. Default: run `go build -buildmode=plugin` with
// stock Go targeting GOOS=linux. Forks that ship a non-stock Go
// toolchain (e.g. microsoft/go under FIPS) override this to build
// inside their toolchain image so the resulting .so matches the alpha
// container's ABI.
var BuildPlugins = defaultBuildPlugins

func defaultComposeArgs(path string) []string {
	return []string{"-f", path}
}

func defaultEnvForCompose() []string {
	return nil
}
