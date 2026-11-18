/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package authorization

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/stretchr/testify/require"
)

// jwkJSON builds the public half of an RSA key of the given size as a JWK.
func jwkJSON(t *testing.T, bits int, kid string) json.RawMessage {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, bits)
	require.NoError(t, err)
	bs, err := (&jose.JSONWebKey{
		Key:       &key.PublicKey,
		KeyID:     kid,
		Algorithm: "RS256",
		Use:       "sig",
	}).MarshalJSON()
	require.NoError(t, err)
	return bs
}

func jwksServer(t *testing.T, keys ...json.RawMessage) *httptest.Server {
	t.Helper()
	body, err := json.Marshal(struct {
		Keys []json.RawMessage `json:"keys"`
	}{Keys: keys})
	require.NoError(t, err)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
}

func metaFor(url string) *AuthMeta {
	meta := &AuthMeta{
		JWKUrls:    []string{url},
		jwkSet:     make([]*jose.JSONWebKeySet, 1),
		expiryTime: make([]time.Time, 1),
	}
	meta.InitHttpClient()
	return meta
}

// go-jose 4.1.6 caps JWK RSA moduli at MaxRSAModulusBits. An IdP that publishes
// one key above the cap must not take the other keys down with it, because
// FetchJWKs failing fails the whole GraphQL schema load.
func TestFetchJWKSkipsUnusableKeys(t *testing.T) {
	require.Equal(t, 4096, jose.MaxRSAModulusBits, "the cap this test is about moved")

	srv := jwksServer(t,
		jwkJSON(t, 2048, "usable"),
		jwkJSON(t, jose.MaxRSAModulusBits+1024, "too-big"),
	)
	defer srv.Close()

	meta := metaFor(srv.URL)
	require.NoError(t, meta.FetchJWK(0))

	keys := meta.jwkSet[0].Keys
	require.Len(t, keys, 1, "the oversized key should have been dropped")
	require.Equal(t, "usable", keys[0].KeyID)
	require.Len(t, meta.jwkSet[0].Key("usable"), 1, "lookup by kid still finds the good key")
	require.Empty(t, meta.jwkSet[0].Key("too-big"), "the dropped key is not resolvable")
}

// Dropping every key would leave a set that silently verifies nothing, so that
// case still fails loudly.
func TestFetchJWKFailsWhenNoKeyIsUsable(t *testing.T) {
	srv := jwksServer(t, jwkJSON(t, jose.MaxRSAModulusBits+1024, "too-big"))
	defer srv.Close()

	err := metaFor(srv.URL).FetchJWK(0)
	require.ErrorContains(t, err, "no usable key in the JWK set")
	require.ErrorContains(t, err, "kid too-big")
}

func TestFetchJWKAcceptsOrdinaryKeySizes(t *testing.T) {
	for _, bits := range []int{2048, 4096} {
		t.Run(fmt.Sprint(bits), func(t *testing.T) {
			srv := jwksServer(t, jwkJSON(t, bits, "sig"))
			defer srv.Close()

			meta := metaFor(srv.URL)
			require.NoError(t, meta.FetchJWK(0))
			require.Len(t, meta.jwkSet[0].Keys, 1)
		})
	}
}

// An empty JWKS is not an error: it was accepted before and an IdP may publish
// one while rotating.
func TestFetchJWKAcceptsEmptyKeySet(t *testing.T) {
	srv := jwksServer(t)
	defer srv.Close()

	meta := metaFor(srv.URL)
	require.NoError(t, meta.FetchJWK(0))
	require.Empty(t, meta.jwkSet[0].Keys)
}
