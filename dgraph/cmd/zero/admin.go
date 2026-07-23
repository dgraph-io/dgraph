/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package zero

import (
	"crypto/subtle"
	"net"
	"net/http"

	"github.com/dgraph-io/dgraph/v25/worker"
	"github.com/dgraph-io/dgraph/v25/x"
)

// remoteHost extracts the source host (IP) from an *http.Request. It handles the "ip:port"
// form used by RemoteAddr as well as a bare address.
func remoteHost(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// adminSecurityConfigured reports whether the operator has configured any admin auth control
// via --security (a poor man's auth token or a source-IP whitelist).
func adminSecurityConfigured() bool {
	return worker.Config.AuthToken != "" || len(x.WorkerConfig.WhiteListedIPRanges) > 0
}

// isAdminRequestAuthorized reports whether r is allowed to invoke a Zero admin HTTP endpoint.
// A request is authorized when it carries the configured poor man's auth token
// (--security "token=...") in the X-Dgraph-AuthToken header, or its source IP is loopback or
// within a configured whitelist range (--security "whitelist=...").
func isAdminRequestAuthorized(r *http.Request) bool {
	if token := worker.Config.AuthToken; token != "" &&
		subtle.ConstantTimeCompare([]byte(token), []byte(r.Header.Get("X-Dgraph-AuthToken"))) == 1 {
		return true
	}
	return x.IsIpWhitelisted(remoteHost(r))
}

// adminAuthHandler guards a Zero admin HTTP endpoint.
//
// Zero's admin endpoints previously carried no authentication at all, so any caller able to
// reach the HTTP port (default 6080) could invoke them. Two tiers of protection apply:
//
//   - strict=true is used for the destructive control-plane endpoints (/removeNode,
//     /moveTablet). These are always guarded: a request must be authorized (token or
//     whitelisted/loopback IP). With neither a token nor a whitelist configured, only loopback
//     is admitted, so a remote caller cannot disrupt the control plane out of the box.
//
//   - strict=false is used for the informational and allocation endpoints (/state, /assign),
//     which existing tooling (dashboards, monitoring, loaders) reads over HTTP. Enforcement is
//     opt-in: auth is applied only once the operator sets a token or whitelist via --security.
//     Until then the request is allowed, preserving prior behavior. Network isolation of the
//     port remains the primary control for these, as it already is for Zero's gRPC surface.
func adminAuthHandler(strict bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		x.AddCorsHeaders(w)
		if r.Method == http.MethodOptions {
			return
		}
		if (strict || adminSecurityConfigured()) && !isAdminRequestAuthorized(r) {
			w.WriteHeader(http.StatusUnauthorized)
			x.SetStatus(w, x.ErrorUnauthorized,
				"Request is not from a whitelisted IP and does not carry a valid X-Dgraph-AuthToken. "+
					`Configure --security "whitelist=...;token=..." on Zero to permit remote admin access.`)
			return
		}
		next.ServeHTTP(w, r)
	})
}
