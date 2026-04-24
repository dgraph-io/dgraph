/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package dgraphtest

import "github.com/docker/docker/api/types/container"

// This file declares public extensibility hooks for the dgraphtest test
// harness. See testutil/hooks.go for the full convention. Each hook is
// initialized to an unexported default that implements upstream behavior;
// a private fork reassigns the public var from its own init().

// ApplyContainerUser optionally sets cfg.User to pin a container to a
// specific UID/GID. Default: no-op (container runs as the image's
// default user). Forks that ship nonroot runtime images and want tests
// to run under the host UID (so bind-mounted paths are writable) set
// cfg.User to "host-uid:host-gid".
var ApplyContainerUser = defaultApplyContainerUser

// WidenTempDirPerms optionally relaxes the 0700 default permissions of
// os.MkdirTemp to allow a container's nonroot user to traverse the
// bind-mounted path. Default: no-op. Forks with nonroot runtime images
// set the mode to 0755.
var WidenTempDirPerms = defaultWidenTempDirPerms

// WidenSecretFilePerms optionally relaxes secret-file permissions from
// the default 0600 to let a container's nonroot user read the file via a
// bind-mount. Default: no-op. Forks with nonroot runtime images set 0644.
// Tempdir isolation bounds the exposure to ephemeral test processes.
var WidenSecretFilePerms = defaultWidenSecretFilePerms

// GeneratePlugins optionally overrides the plugin-build flow used by
// LocalCluster.GeneratePlugins. If handled=true, the caller uses the
// returned tokenizers string to populate the cluster's custom_tokenizers
// arg and skips the default native-go-build path. If handled=false, the
// caller falls through to the default in-process build. Default:
// returns ("", false, nil) — delegate to upstream.
//
// Params:
//   - raceEnabled: forward the --race flag to the plugin build
//   - tempBinDir: the cluster's per-test tempdir (host-side); a plugins/
//     subdirectory will be created there and bind-mounted into the
//     container-side build environment
//   - repoRoot: host path to the dgraph repo root, mounted so the
//     builder sees testutil/custom_plugins/... source files
//
// The returned tokenizers string is the comma-separated list of .so
// paths as seen by the alpha container at runtime (typically under
// /gobin/plugins/…).
var GeneratePlugins = defaultGeneratePlugins

// SetupLinuxBinaries optionally overrides the dgraph-binary staging
// performed by LocalCluster.setupBinary for the Linux branch. If
// handled=true, the caller returns err without executing the upstream
// single-binary Linux path. Default: (false, nil) — delegate to
// upstream.
var SetupLinuxBinaries = defaultSetupLinuxBinaries

// HostBinaryName optionally overrides the filename of the host-native
// dgraph binary that LocalCluster.HostDgraphBinaryPath joins with
// tempBinDir. Default: empty string (use upstream default path selection
// based on runtime.GOOS). Forks may return e.g. "dgraph_host" to name a
// separate host-native binary alongside a container-only binary.
var HostBinaryName = defaultHostBinaryName

func defaultApplyContainerUser(cfg *container.Config) {}
func defaultWidenTempDirPerms(path string) error      { return nil }
func defaultWidenSecretFilePerms(path string) error   { return nil }
func defaultHostBinaryName() string                   { return "" }
func defaultSetupLinuxBinaries(tempBinDir, version string) (handled bool, err error) {
	return false, nil
}
func defaultGeneratePlugins(raceEnabled bool, tempBinDir, repoRoot string) (tokenizers string, handled bool, err error) {
	return "", false, nil
}
