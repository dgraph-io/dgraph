/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package main

// This file declares public extensibility hooks for the t test runner.
// See testutil/hooks.go for the full convention.

// EnvForCompose returns extra KEY=VALUE entries to inject into the
// environment of docker-compose subprocesses that t spawns. Default:
// nil. Forks may inject UID/GID so `${UID:-65532}` in generated compose
// files resolves to the host UID rather than the image's nonroot user.
var EnvForCompose = defaultEnvForCompose

// ComposeFileArgs returns the file-selector args docker-compose receives
// for the given compose-file path. The baseDir argument is the repo
// root (t's --base flag), passed explicitly so the hook is pure and
// shares no package state with t/t.go. Default: returns "-f <path>"
// unchanged, matching upstream behavior where the pristine compose file
// passes straight through.
//
// A fork may override this to layer additional `-f <overlay>` files,
// switch the compose-file path, or add `--project-directory <dir>` so
// relative bind-mount sources resolve against the pristine test package
// dir rather than wherever the override emits its base file.
var ComposeFileArgs = defaultComposeFileArgs

func defaultEnvForCompose() []string { return nil }

// defaultComposeFileArgs ignores baseDir; it exists in the signature so
// overrides that need it (to resolve paths relative to the repo root)
// do not have to thread it through some other channel.
func defaultComposeFileArgs(path, _ string) []string {
	return []string{"-f", path}
}
