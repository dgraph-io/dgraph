/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package main

// This file declares public extensibility hooks for the t test runner.
// See testutil/hooks.go for the full convention.

// EnvForCompose returns extra KEY=VALUE entries to inject into the
// environment of docker-compose subprocesses that t spawns. Default:
// nil. Forks may inject e.g. UID/GID so `${UID:-65532}` in generated
// compose files resolves to the host UID rather than the image's
// nonroot user.
var EnvForCompose = defaultEnvForCompose

// ComposeFileArgs returns the file-selector args docker-compose receives
// for the given compose-file path. The baseDir argument is the repo root
// (t's --base flag), passed explicitly so the hook is pure — no package
// state shared with t/t.go. Default: returns "-f <path>" unchanged,
// matching upstream behavior where the pristine compose file is passed
// straight through.
//
// A fork that runs a compose-file generator may override this to rewrite
// the path to a generated overlay (e.g. under $DGRAPH_COMPOSE_BUILD_DIR)
// and add "--project-directory <original-dir>" so relative bind-mount
// sources resolve against the pristine test package dir rather than the
// overlay output dir. Unknown paths or an empty $DGRAPH_COMPOSE_BUILD_DIR
// fall through to the upstream-compatible "-f <path>" form.
var ComposeFileArgs = defaultComposeFileArgs

func defaultEnvForCompose() []string { return nil }

// defaultComposeFileArgs ignores baseDir; it only exists in the signature so
// overrides that need it (for resolving paths relative to the repo root) do
// not have to thread it through some other channel.
func defaultComposeFileArgs(path, _ string) []string {
	return []string{"-f", path}
}
