/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package posting

// Read-your-writes for the vector index transaction view: an HNSW insert
// reads neighbor adjacency rows and writes them back through the same
// transaction; a later insert in the SAME transaction must see the earlier
// insert's update, or its rewrite silently drops the earlier links and
// orphans vectors. This bites any batched vector mutation (N vectors in one
// mutation = N index inserts sharing one transaction) and the rebuild drain.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/v25/tok/index"
	"github.com/dgraph-io/dgraph/v25/x"
)

func TestViTxnReadYourWritesSameKey(t *testing.T) {
	ctx := context.Background()
	attr := x.AttrInRootNamespace("vryw__vector_")
	key := x.DataKey(attr, 7)

	startTs := uint64(910000) // isolated ts range for this test
	txn := Oracle().RegisterStartTs(startTs)

	vt := NewViTxn(txn)

	write := func(val string) {
		require.NoError(t, vt.AddMutation(ctx, key, &index.KeyValue{
			Entity: 7, Attr: attr, Value: []byte(val),
		}))
	}
	read := func() string {
		got, err := vt.Get(key)
		require.NoError(t, err)
		return string(got)
	}

	write("v1")
	require.Equal(t, "v1", read(), "first write must be visible in-txn")

	write("v2")
	require.Equal(t, "v2", read(),
		"second write in the same txn must shadow the first (read-your-writes)")

	write("v3")
	require.Equal(t, "v3", read(),
		"third write in the same txn must shadow earlier ones (read-your-writes)")
}
