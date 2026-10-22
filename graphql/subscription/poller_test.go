/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package subscription

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"

	"github.com/dgraph-io/dgraph/v25/x"
)

// recordingAuthenticator reports what credential metadata reached it, so the test
// observes the context subscriberContext built without depending on how any real
// authenticator verifies a credential.
type recordingAuthenticator struct{}

func (recordingAuthenticator) Name() string { return "recording" }

func (recordingAuthenticator) Authenticate(ctx context.Context) (*x.Principal, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	if t := md.Get("auth-token"); len(t) > 0 {
		return &x.Principal{Subject: "token:" + t[0], Method: x.MethodPreshared}, nil
	}
	if t := md.Get("accessJwt"); len(t) > 0 {
		return &x.Principal{Subject: "jwt:" + t[0], Method: x.MethodACL}, nil
	}
	return nil, nil
}

// headerWith builds a stored header the way the subscription handler does, with
// Set, so the key goes through the same canonicalization a real request's would.
func headerWith(key, value string) http.Header {
	h := http.Header{}
	h.Set(key, value)
	return h
}

// TestSubscriberContextResolvesIdentity pins the fix for a gap a reviewer found:
// the poller attached only the access JWT, so the --security auth token never
// reached the context and no Principal was resolved for any subscriber. Under
// --security "anonymous=none" that refused every poll, authenticated or not.
func TestSubscriberContextResolvesIdentity(t *testing.T) {
	x.SetAuthenticator(recordingAuthenticator{})
	t.Cleanup(func() { x.SetAuthenticator(nil) })

	tests := []struct {
		name        string
		header      http.Header
		wantSubject string
	}{{
		name:        "the --security auth token reaches the authenticator",
		header:      headerWith("X-Dgraph-AuthToken", "s3cr3t"),
		wantSubject: "token:s3cr3t",
	}, {
		name:        "the ACL access JWT still reaches it",
		header:      headerWith("X-Dgraph-AccessToken", "a.b.c"),
		wantSubject: "jwt:a.b.c",
	}, {
		// A subscriber that presented nothing is anonymous, and must stay so: this
		// path must not invent an identity.
		name:   "no credential, no Principal",
		header: http.Header{},
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := x.PrincipalFrom(subscriberContext(tt.header))
			if tt.wantSubject == "" {
				require.Nil(t, p)
				return
			}
			require.NotNil(t, p, "the stored header must resolve to a Principal")
			require.Equal(t, tt.wantSubject, p.Subject)
		})
	}
}
