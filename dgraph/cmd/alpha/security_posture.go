/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package alpha

import (
	"fmt"

	"github.com/dgraph-io/dgraph/v25/x"
)

// securityWarnings returns the startup warnings for a --security configuration
// whose parts do not add up, or nil when they do.
//
// Three controls gate Alpha's privileged operations -- the whitelist, the auth
// token, and ACL -- and each one passes when its own feature is unconfigured. The
// protection an operator gets is whatever they configured rather than the union of
// the three, and the two combinations below are the ones where that produces an
// outcome they almost certainly did not intend.
//
// It is pure and returns strings rather than logging, so the combinations can be
// pinned by a table test.
func securityWarnings(posture x.AnonymousPosture, whitelist string, ips []x.IPRange,
	authToken string, aclEnabled bool, httpPort int) []string {

	hasCredential := aclEnabled || authToken != ""
	var out []string

	// The shipped default is an empty whitelist, which admits loopback only, so the
	// admin plane does not leave the host without an operator widening it. This
	// warns at exactly that point: whitelisting answers where a request came from
	// and has no credential in it, so a widened range with nothing else configured
	// means every address inside it can run privileged operations anonymously.
	if posture == x.AnonymousFull && len(ips) > 0 && !hasCredential {
		out = append(out, fmt.Sprintf(
			`SECURITY: --security "whitelist=%s" admits non-loopback callers, but neither ACL nor `+
				`an admin token is configured. Privileged operations (backup, restore, export, `+
				`shutdown, removeNode, moveTablet, assign, draining, config, namespace create and `+
				`drop) are reachable from that range WITHOUT ANY CREDENTIAL: anyone who can reach `+
				`port %d can read the whole database via backup or export, restore over it, or shut `+
				`the cluster down. Set --security "token=..." or enable ACL, and narrow the `+
				`whitelist to the addresses that actually administer this cluster. `+
				`--security "anonymous=data" additionally denies every administrative operation to `+
				`a caller that presents no credential.`, whitelist, httpPort))
	}

	// A closed posture with nothing that can produce an identity. Every capability
	// check will deny, including the operator's own, so the cluster cannot be
	// administered at all. This is a misconfiguration rather than a hardening, and
	// it is worth saying so loudly at boot instead of letting it surface as a
	// permission error during an incident.
	if posture.RequiresIdentityForCapability() && !hasCredential {
		out = append(out, fmt.Sprintf(
			`SECURITY: --security "anonymous=%s" requires an identified caller, but neither ACL nor `+
				`an admin token is configured, so no request can ever be identified. Every `+
				`administrative operation will be denied, including from loopback, and this cluster `+
				`cannot be administered. Set --security "token=..." or enable ACL.`, posture))
	}

	return out
}
