/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package dgraphtest

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestZeroCmdCarriesSecurityWhitelist pins that the harness configures Zero's admin
// whitelist, not only the Alpha's.
//
// Zero guards /moveTablet and /removeNode with adminAuthHandler(strict=true), which
// applies whether or not --security is configured: with neither a token nor a
// whitelist, only a loopback caller is admitted. A test process reaching Zero through
// a published container port is not loopback, so a harness that omits this cannot
// exercise those endpoints at all — every call returns 401 ErrorUnauthorized.
//
// This is asserted on the COMMAND rather than end to end, deliberately. Whether the
// container observes a loopback peer depends on the host's Docker networking: on Linux
// a published port arrives from the bridge gateway and the guard fires, while on
// Docker Desktop it can arrive as loopback and the guard never fires. An integration
// test would therefore pass on some developer machines no matter what the harness
// passes, which is worse than no test. The guard's own behaviour is already covered by
// the adminAuthHandler unit tests in dgraph/cmd/zero.
func TestZeroCmdCarriesSecurityWhitelist(t *testing.T) {
	c := &LocalCluster{conf: NewClusterConfig()}
	z := &zero{id: 0, aliasName: "zero0"}

	cmd := strings.Join(z.cmd(c), " ")
	require.Contains(t, cmd, "--security=whitelist=",
		"Zero must get an admin whitelist, or the harness cannot reach /moveTablet and "+
			"/removeNode: they are guarded with strict=true, so without it only a loopback "+
			"caller is admitted")
}

// TestZeroCmdCarriesSecurityToken: when a test configures a token, Zero has to know it
// too. Zero authorizes an admin request on the token OR the whitelist, so a cluster
// where only the Alpha knows the token is incoherent rather than merely stricter.
func TestZeroCmdCarriesSecurityToken(t *testing.T) {
	const token = "shhh"
	c := &LocalCluster{conf: NewClusterConfig().WithSecurityToken(token)}
	z := &zero{id: 0, aliasName: "zero0"}

	cmd := strings.Join(z.cmd(c), " ")
	require.Contains(t, cmd, "token="+token,
		"Zero must be told the configured --security token, as the Alpha already is")
}

// TestZeroCmdPreV21OmitsSecurity. Pre-v21 binaries predate both the --security
// superflag and the admin guard, so passing the flag there would fail an upgrade test
// with an unknown-flag error while fixing nothing. The Alpha's command makes the same
// split for the same reason.
func TestZeroCmdPreV21OmitsSecurity(t *testing.T) {
	c := &LocalCluster{conf: NewClusterConfig(), lowerThanV21: true}
	z := &zero{id: 0, aliasName: "zero0"}

	cmd := strings.Join(z.cmd(c), " ")
	require.NotContains(t, cmd, "--security",
		"a pre-v21 Zero does not understand --security and would fail to start")
}
