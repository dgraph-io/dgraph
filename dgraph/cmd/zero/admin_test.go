/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package zero

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/v25/worker"
	"github.com/dgraph-io/dgraph/v25/x"
)

// sentinelHandler stands in for Zero's admin handlers (/removeNode, /moveTablet, /assign,
// /state). It records whether the request was allowed to reach the underlying handler.
func sentinelHandler(reached *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*reached = true
		w.WriteHeader(http.StatusOK)
	})
}

// resetSecurityConfig clears the process-wide auth configuration so each test starts from the
// default (no token, no whitelist) and restores it afterwards.
func resetSecurityConfig(t *testing.T) {
	t.Helper()
	prevToken := worker.Config.AuthToken
	prevRanges := x.WorkerConfig.WhiteListedIPRanges
	worker.Config.AuthToken = ""
	x.WorkerConfig.WhiteListedIPRanges = nil
	t.Cleanup(func() {
		worker.Config.AuthToken = prevToken
		x.WorkerConfig.WhiteListedIPRanges = prevRanges
	})
}

func doAdminRequest(t *testing.T, strict bool, remoteAddr, token string) (*httptest.ResponseRecorder, bool) {
	t.Helper()
	reached := false
	handler := adminAuthHandler(strict, sentinelHandler(&reached))

	req := httptest.NewRequest(http.MethodGet, "/removeNode?id=1&group=0", nil)
	req.RemoteAddr = remoteAddr
	if token != "" {
		req.Header.Set("X-Dgraph-AuthToken", token)
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr, reached
}

// TestZeroAdminAuth_StrictRemoteRejectedByDefault is the core regression test for
// GHSA-qj98-gp63-54xj. For the destructive endpoints (/removeNode, /moveTablet), with no token
// and no whitelist configured (the default), a remote unauthenticated caller must not reach
// the handler.
func TestZeroAdminAuth_StrictRemoteRejectedByDefault(t *testing.T) {
	resetSecurityConfig(t)

	rr, reached := doAdminRequest(t, true, "8.8.8.8:5555", "")
	require.False(t, reached, "remote unauthenticated request reached the destructive handler")
	require.Equal(t, http.StatusUnauthorized, rr.Code)
}

// TestZeroAdminAuth_StrictLoopbackAllowed verifies that local administration keeps working:
// a loopback request to a destructive endpoint is always allowed.
func TestZeroAdminAuth_StrictLoopbackAllowed(t *testing.T) {
	resetSecurityConfig(t)

	for _, addr := range []string{"127.0.0.1:5555", "[::1]:5555"} {
		rr, reached := doAdminRequest(t, true, addr, "")
		require.True(t, reached, "loopback request %q was rejected", addr)
		require.Equal(t, http.StatusOK, rr.Code)
	}
}

// TestZeroAdminAuth_NonStrictOpenByDefault verifies backward compatibility for the
// informational/allocation endpoints (/state, /assign): with no token or whitelist configured,
// a remote caller is still allowed, so existing tooling and monitoring keep working.
func TestZeroAdminAuth_NonStrictOpenByDefault(t *testing.T) {
	resetSecurityConfig(t)

	rr, reached := doAdminRequest(t, false, "8.8.8.8:5555", "")
	require.True(t, reached, "remote request to a non-strict endpoint was rejected by default")
	require.Equal(t, http.StatusOK, rr.Code)
}

// TestZeroAdminAuth_NonStrictEnforcedWhenConfigured verifies that once an operator opts in
// (here, by setting a token), the informational/allocation endpoints are enforced too.
func TestZeroAdminAuth_NonStrictEnforcedWhenConfigured(t *testing.T) {
	resetSecurityConfig(t)
	worker.Config.AuthToken = "s3cr3t"

	rr, reached := doAdminRequest(t, false, "8.8.8.8:5555", "")
	require.False(t, reached, "remote request to a non-strict endpoint was admitted despite configured token")
	require.Equal(t, http.StatusUnauthorized, rr.Code)

	rr, reached = doAdminRequest(t, false, "8.8.8.8:5555", "s3cr3t")
	require.True(t, reached, "remote request with valid token was rejected")
	require.Equal(t, http.StatusOK, rr.Code)
}

// TestZeroAdminAuth_TokenAllowsRemote verifies the poor man's auth token escape hatch on the
// strict (destructive) endpoints: a remote caller must present the exact token to be admitted.
func TestZeroAdminAuth_TokenAllowsRemote(t *testing.T) {
	resetSecurityConfig(t)
	worker.Config.AuthToken = "s3cr3t"

	rr, reached := doAdminRequest(t, true, "8.8.8.8:5555", "s3cr3t")
	require.True(t, reached, "remote request with valid token was rejected")
	require.Equal(t, http.StatusOK, rr.Code)

	rr, reached = doAdminRequest(t, true, "8.8.8.8:5555", "wrong")
	require.False(t, reached, "remote request with wrong token was admitted")
	require.Equal(t, http.StatusUnauthorized, rr.Code)

	rr, reached = doAdminRequest(t, true, "8.8.8.8:5555", "")
	require.False(t, reached, "remote request with no token was admitted")
	require.Equal(t, http.StatusUnauthorized, rr.Code)
}

// TestZeroAdminAuth_WhitelistAllowsRemote verifies the IP whitelist escape hatch on the strict
// (destructive) endpoints.
func TestZeroAdminAuth_WhitelistAllowsRemote(t *testing.T) {
	resetSecurityConfig(t)
	ranges, err := x.GetIPsFromString("8.8.8.0/24")
	require.NoError(t, err)
	x.WorkerConfig.WhiteListedIPRanges = ranges

	rr, reached := doAdminRequest(t, true, "8.8.8.8:5555", "")
	require.True(t, reached, "whitelisted remote request was rejected")
	require.Equal(t, http.StatusOK, rr.Code)

	rr, reached = doAdminRequest(t, true, "9.9.9.9:5555", "")
	require.False(t, reached, "non-whitelisted remote request was admitted")
	require.Equal(t, http.StatusUnauthorized, rr.Code)
}
