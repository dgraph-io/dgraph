/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package x

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseTabletPlacement(t *testing.T) {
	parse := func(doc string) ([]TabletPlacement, error) {
		return ParseTabletPlacement(strings.NewReader(doc))
	}

	t.Run("happy path", func(t *testing.T) {
		entries, err := parse(`[
			{"predicate": "name", "group": 1},
			{"predicate": "payload", "group": 2, "namespace": 0},
			{"predicate": "payload", "group": 3, "namespace": 5}
		]`)
		require.NoError(t, err)
		require.Equal(t, []TabletPlacement{
			{Predicate: "name", Group: 1, Namespace: 0},
			{Predicate: "payload", Group: 2, Namespace: 0},
			{Predicate: "payload", Group: 3, Namespace: 5},
		}, entries)
	})

	t.Run("missing namespace defaults to root", func(t *testing.T) {
		entries, err := parse(`[{"predicate": "name", "group": 2}]`)
		require.NoError(t, err)
		require.Equal(t, RootNamespace, entries[0].Namespace)
	})

	t.Run("unknown field rejected", func(t *testing.T) {
		_, err := parse(`[{"predicate": "name", "group": 1, "shard": 0}]`)
		require.ErrorContains(t, err, "shard")
	})

	t.Run("empty predicate rejected", func(t *testing.T) {
		_, err := parse(`[{"predicate": "", "group": 1}]`)
		require.ErrorContains(t, err, "empty predicate")
	})

	t.Run("group zero rejected", func(t *testing.T) {
		_, err := parse(`[{"predicate": "name", "group": 0}]`)
		require.ErrorContains(t, err, "group must be >= 1")
	})

	t.Run("reserved predicate rejected", func(t *testing.T) {
		_, err := parse(`[{"predicate": "dgraph.type", "group": 2}]`)
		require.ErrorContains(t, err, "reserved")
	})

	t.Run("namespaced predicate spelling rejected", func(t *testing.T) {
		_, err := parse(`[{"predicate": "0-name", "group": 1}]`)
		require.ErrorContains(t, err, "namespace field")
	})

	t.Run("duplicate entry rejected, same predicate across namespaces allowed", func(t *testing.T) {
		_, err := parse(`[
			{"predicate": "name", "group": 1, "namespace": 5},
			{"predicate": "name", "group": 2, "namespace": 5}
		]`)
		require.ErrorContains(t, err, "duplicate")

		_, err = parse(`[
			{"predicate": "name", "group": 1, "namespace": 5},
			{"predicate": "name", "group": 2, "namespace": 6}
		]`)
		require.NoError(t, err)
	})

	t.Run("all faults reported at once", func(t *testing.T) {
		_, err := parse(`[
			{"predicate": "", "group": 0},
			{"predicate": "dgraph.xid", "group": 2},
			{"predicate": "name", "group": 1},
			{"predicate": "name", "group": 3}
		]`)
		require.Error(t, err)
		for _, want := range []string{"empty predicate", "group must be >= 1", "reserved", "duplicate"} {
			require.ErrorContains(t, err, want)
		}
	})

	t.Run("malformed json", func(t *testing.T) {
		_, err := parse(`{"predicate": "name"}`)
		require.ErrorContains(t, err, "parsing tablet placement")
		_, err = parse(`[{"predicate": "name", "group": 1}] trailing`)
		require.ErrorContains(t, err, "trailing data")
	})
}

func TestParseTabletPlacementFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "placement.json")
	require.NoError(t, os.WriteFile(path,
		[]byte(`[{"predicate": "payload", "group": 2}]`), 0600))

	entries, err := ParseTabletPlacementFile(path)
	require.NoError(t, err)
	require.Equal(t, []TabletPlacement{{Predicate: "payload", Group: 2}}, entries)

	_, err = ParseTabletPlacementFile(filepath.Join(t.TempDir(), "nope.json"))
	require.ErrorContains(t, err, "nope.json")
}
