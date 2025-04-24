/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package x

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNameSpace(t *testing.T) {
	ns := uint64(133)
	attr := "name"
	nsAttr := NamespaceAttr(ns, attr)
	parsedNs, parsedAttr := ParseNamespaceAttr(nsAttr)
	require.Equal(t, ns, parsedNs)
	require.Equal(t, attr, parsedAttr)
}

func TestDataKey(t *testing.T) {
	var uid uint64

	// key with uid = 0 is invalid
	uid = 0
	key := DataKey(AttrInRootNamespace("bad uid"), uid)
	_, err := Parse(key)
	require.Error(t, err)

	for uid = 1; uid < 1001; uid++ {
		// Use the uid to derive the attribute so it has variable length and the test
		// can verify that multiple sizes of attr work correctly.
		sattr := fmt.Sprintf("attr:%d", uid)
		key := DataKey(AttrInRootNamespace(sattr), uid)
		pk, err := Parse(key)
		require.NoError(t, err)

		require.True(t, pk.IsData())
		require.Equal(t, sattr, ParseAttr(pk.Attr))
		require.Equal(t, uid, pk.Uid)
		require.Equal(t, uint64(0), pk.StartUid)
	}

	keys := make([]string, 0, 1024)
	for uid = 1024; uid >= 1; uid-- {
		key := DataKey(AttrInRootNamespace("testing.key"), uid)
		keys = append(keys, string(key))
	}
	// Test that sorting is as expected.
	sort.Strings(keys)
	require.True(t, sort.StringsAreSorted(keys))
	for i, key := range keys {
		exp := DataKey(AttrInRootNamespace("testing.key"), uint64(i+1))
		require.Equal(t, string(exp), key)
	}
}

func TestParseDataKeyWithStartUid(t *testing.T) {
	var uid uint64
	startUid := uint64(math.MaxUint64)
	for uid = 1; uid < 1001; uid++ {
		sattr := fmt.Sprintf("attr:%d", uid)
		key := DataKey(AttrInRootNamespace(sattr), uid)
		key, err := SplitKey(key, startUid)
		require.NoError(t, err)
		pk, err := Parse(key)
		require.NoError(t, err)

		require.True(t, pk.IsData())
		require.Equal(t, sattr, ParseAttr(pk.Attr))
		require.Equal(t, uid, pk.Uid)
		require.Equal(t, pk.HasStartUid, true)
		require.Equal(t, startUid, pk.StartUid)
	}
}

func TestIndexKey(t *testing.T) {
	for uid := range 1001 {
		sattr := fmt.Sprintf("attr:%d", uid)
		sterm := fmt.Sprintf("term:%d", uid)

		key := IndexKey(AttrInRootNamespace(sattr), sterm)
		pk, err := Parse(key)
		require.NoError(t, err)

		require.True(t, pk.IsIndex())
		require.Equal(t, sattr, ParseAttr(pk.Attr))
		require.Equal(t, sterm, pk.Term)
	}
}

func TestIndexKeyWithStartUid(t *testing.T) {
	startUid := uint64(math.MaxUint64)
	for uid := range 1001 {
		sattr := fmt.Sprintf("attr:%d", uid)
		sterm := fmt.Sprintf("term:%d", uid)

		key := IndexKey(AttrInRootNamespace(sattr), sterm)
		key, err := SplitKey(key, startUid)
		require.NoError(t, err)
		pk, err := Parse(key)
		require.NoError(t, err)

		require.True(t, pk.IsIndex())
		require.Equal(t, sattr, ParseAttr(pk.Attr))
		require.Equal(t, sterm, pk.Term)
		require.Equal(t, pk.HasStartUid, true)
		require.Equal(t, startUid, pk.StartUid)
	}
}

func TestReverseKey(t *testing.T) {
	var uid uint64
	for uid = 1; uid < 1001; uid++ {
		sattr := fmt.Sprintf("attr:%d", uid)

		key := ReverseKey(AttrInRootNamespace(sattr), uid)
		pk, err := Parse(key)
		require.NoError(t, err)

		require.True(t, pk.IsReverse())
		require.Equal(t, sattr, ParseAttr(pk.Attr))
		require.Equal(t, uid, pk.Uid)
	}
}

func TestReverseKeyWithStartUid(t *testing.T) {
	var uid uint64
	startUid := uint64(math.MaxUint64)
	for uid = 1; uid < 1001; uid++ {
		sattr := fmt.Sprintf("attr:%d", uid)

		key := ReverseKey(AttrInRootNamespace(sattr), uid)
		key, err := SplitKey(key, startUid)
		require.NoError(t, err)
		pk, err := Parse(key)
		require.NoError(t, err)

		require.True(t, pk.IsReverse())
		require.Equal(t, sattr, ParseAttr(pk.Attr))
		require.Equal(t, uid, pk.Uid)
		require.Equal(t, pk.HasStartUid, true)
		require.Equal(t, startUid, pk.StartUid)
	}
}

func TestCountKey(t *testing.T) {

	for count := range uint32(1001) {
		sattr := fmt.Sprintf("attr:%d", count)

		key := CountKey(AttrInRootNamespace(sattr), count, true)
		pk, err := Parse(key)
		require.NoError(t, err)

		require.True(t, pk.IsCountOrCountRev())
		require.Equal(t, sattr, ParseAttr(pk.Attr))
		require.Equal(t, count, pk.Count)
	}
}

func TestCountKeyWithStartUid(t *testing.T) {
	startUid := uint64(math.MaxUint64)
	for count := range uint32(1001) {
		sattr := fmt.Sprintf("attr:%d", count)

		key := CountKey(AttrInRootNamespace(sattr), count, true)
		key, err := SplitKey(key, startUid)
		require.NoError(t, err)
		pk, err := Parse(key)
		require.NoError(t, err)

		require.True(t, pk.IsCountOrCountRev())
		require.Equal(t, sattr, ParseAttr(pk.Attr))
		require.Equal(t, count, pk.Count)
		require.Equal(t, pk.HasStartUid, true)
		require.Equal(t, startUid, pk.StartUid)
	}
}

func TestSchemaKey(t *testing.T) {
	for uid := range 1001 {
		sattr := fmt.Sprintf("attr:%d", uid)

		key := SchemaKey(AttrInRootNamespace(sattr))
		pk, err := Parse(key)
		require.NoError(t, err)

		require.True(t, pk.IsSchema())
		require.Equal(t, sattr, ParseAttr(pk.Attr))
	}
}

func TestTypeKey(t *testing.T) {
	for uid := range 1001 {
		sattr := fmt.Sprintf("attr:%d", uid)

		key := TypeKey(AttrInRootNamespace(sattr))
		pk, err := Parse(key)
		require.NoError(t, err)

		require.True(t, pk.IsType())
		require.Equal(t, sattr, ParseAttr(pk.Attr))
	}
}

func TestBadStartUid(t *testing.T) {
	testKey := func(key []byte) {
		key, err := SplitKey(key, 10)
		require.NoError(t, err)
		_, err = Parse(key)
		require.NoError(t, err)
		key = append(key, 0)
		_, err = Parse(key)
		require.Error(t, err)
	}

	key := DataKey(AttrInRootNamespace("aa"), 1)
	testKey(key)

	key = ReverseKey(AttrInRootNamespace("aa"), 1)
	testKey(key)

	key = CountKey(AttrInRootNamespace("aa"), 0, false)
	testKey(key)

	key = CountKey(AttrInRootNamespace("aa"), 0, true)
	testKey(key)
}

func TestBadKeys(t *testing.T) {
	// 0-len key
	key := []byte{}
	_, err := Parse(key)
	require.Error(t, err)

	// key of len < 3
	key = []byte{1}
	_, err = Parse(key)
	require.Error(t, err)

	key = []byte{1, 2}
	_, err = Parse(key)
	require.Error(t, err)

	// key of len < sz (key[1], key[2])
	key = []byte{1, 0x00, 0x04, 1, 2}
	_, err = Parse(key)
	require.Error(t, err)

	// key with uid = 0 is invalid
	uid := 0
	key = DataKey(AttrInRootNamespace("bad uid"), uint64(uid))
	_, err = Parse(key)
	require.Error(t, err)
}

func TestJsonMarshal(t *testing.T) {
	type predicate struct {
		Predicate string `json:"predicate,omitempty"`
	}

	p := &predicate{Predicate: NamespaceAttr(129, "name")}
	b, err := json.Marshal(p)
	require.NoError(t, err)

	var p2 predicate
	require.NoError(t, json.Unmarshal(b, &p2))
	ns, attr := ParseNamespaceAttr(p2.Predicate)
	require.Equal(t, uint64(129), ns)
	require.Equal(t, "name", attr)
}

func TestNsSeparator(t *testing.T) {
	uid := uint64(10)
	pred := "name" + NsSeparator + "surname"
	key := DataKey(AttrInRootNamespace(pred), uid)
	pk, err := Parse(key)
	require.NoError(t, err)
	require.Equal(t, uid, pk.Uid)
	ns, attr := ParseNamespaceAttr(pk.Attr)
	require.Equal(t, RootNamespace, ns)
	require.Equal(t, pred, attr)
}
