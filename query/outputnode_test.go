/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */
package query

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/types"
)

func makeFastJsonNode() *fastJsonNode {
	return &fastJsonNode{}
}

func TestEncodeMemory(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestEncodeMemory")
	}
	var wg sync.WaitGroup

	for x := 0; x < runtime.NumCPU(); x++ {
		n := makeFastJsonNode()
		require.NotNil(t, n)
		for i := 0; i < 15000; i++ {
			n.AddValue(fmt.Sprintf("very long attr name %06d", i), types.ValueForType(types.StringID))
			n.AddListChild(fmt.Sprintf("another long child %06d", i), &fastJsonNode{})
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				n.encode(bufio.NewWriter(ioutil.Discard))
			}
		}()
	}

	wg.Wait()
}

func TestNormalizePBLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestNormalizePBLimit")
	}

	n := (&protoNode{}).New("root")
	require.NotNil(t, n)
	for i := 0; i < 1000; i++ {
		n.AddValue(fmt.Sprintf("very long attr name %06d", i),
			types.ValueForType(types.StringID))
		child1 := n.New("child1")
		n.AddListChild("child1", child1)
		for j := 0; j < 100; j++ {
			child1.AddValue(fmt.Sprintf("long child1 attr %06d", j),
				types.ValueForType(types.StringID))
		}
		child2 := n.New("child2")
		n.AddListChild("child2", child2)
		for j := 0; j < 100; j++ {
			child2.AddValue(fmt.Sprintf("long child2 attr %06d", j),
				types.ValueForType(types.StringID))
		}
		child3 := n.New("child3")
		n.AddListChild("child3", child3)
		for j := 0; j < 100; j++ {
			child3.AddValue(fmt.Sprintf("long child3 attr %06d", j),
				types.ValueForType(types.StringID))
		}
	}
	_, err := n.(*protoNode).normalize()
	require.Error(t, err, "Couldn't evaluate @normalize directive - to many results")
}

func TestNormalizeJSONLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestNormalizeJSONLimit")
	}

	n := (&fastJsonNode{}).New("root")
	require.NotNil(t, n)
	for i := 0; i < 1000; i++ {
		n.AddValue(fmt.Sprintf("very long attr name %06d", i),
			types.ValueForType(types.StringID))
		child1 := n.New("child1")
		n.AddListChild("child1", child1)
		for j := 0; j < 100; j++ {
			child1.AddValue(fmt.Sprintf("long child1 attr %06d", j),
				types.ValueForType(types.StringID))
		}
		child2 := n.New("child2")
		n.AddListChild("child2", child2)
		for j := 0; j < 100; j++ {
			child2.AddValue(fmt.Sprintf("long child2 attr %06d", j),
				types.ValueForType(types.StringID))
		}
		child3 := n.New("child3")
		n.AddListChild("child3", child3)
		for j := 0; j < 100; j++ {
			child3.AddValue(fmt.Sprintf("long child3 attr %06d", j),
				types.ValueForType(types.StringID))
		}
	}
	_, err := n.(*fastJsonNode).normalize()
	require.Error(t, err, "Couldn't evaluate @normalize directive - to many results")
}

func TestNormalizeJSONUid1(t *testing.T) {
	n := (&fastJsonNode{}).New("root")
	require.NotNil(t, n)
	child1 := n.New("child1")
	child1.SetUID(uint64(1), "_uid_")
	child1.AddValue("attr1", types.ValueForType(types.StringID))
	n.AddListChild("child1", child1)

	child2 := n.New("child2")
	child2.SetUID(uint64(2), "_uid_")
	child2.AddValue("attr2", types.ValueForType(types.StringID))
	child1.AddListChild("child2", child2)

	child3 := n.New("child3")
	child3.SetUID(uint64(3), "_uid_")
	child3.AddValue("attr3", types.ValueForType(types.StringID))
	child2.AddListChild("child3", child3)

	normalized, err := n.(*fastJsonNode).normalize()
	require.NoError(t, err)
	require.NotNil(t, normalized)
	nn := (&fastJsonNode{}).New("root")
	for _, c := range normalized {
		nn.AddListChild("alias", &fastJsonNode{attrs: c})
	}

	var b bytes.Buffer
	buf := bufio.NewWriter(&b)
	nn.(*fastJsonNode).encode(buf)
	buf.Flush()
	require.Equal(t, `{"alias":[{"_uid_":"0x3","attr1":"","attr2":"","attr3":""}]}`, b.String())
}

func TestNormalizeJSONUid2(t *testing.T) {
	n := (&fastJsonNode{}).New("root")
	require.NotNil(t, n)
	child1 := n.New("child1")
	child1.SetUID(uint64(1), "_uid_")
	child1.AddValue("___attr1", types.ValueForType(types.StringID))
	n.AddListChild("child1", child1)

	child2 := n.New("child2")
	child2.SetUID(uint64(2), "_uid_")
	child2.AddValue("___attr2", types.ValueForType(types.StringID))
	child1.AddListChild("child2", child2)

	child3 := n.New("child3")
	child3.SetUID(uint64(3), "_uid_")
	child3.AddValue(fmt.Sprintf("attr3"), types.ValueForType(types.StringID))
	child2.AddListChild("child3", child3)

	normalized, err := n.(*fastJsonNode).normalize()
	require.NoError(t, err)
	require.NotNil(t, normalized)
	nn := (&fastJsonNode{}).New("root")
	for _, c := range normalized {
		nn.AddListChild("alias", &fastJsonNode{attrs: c})
	}

	var b bytes.Buffer
	buf := bufio.NewWriter(&b)
	nn.(*fastJsonNode).encode(buf)
	buf.Flush()
	require.Equal(t, `{"alias":[{"___attr1":"","___attr2":"","_uid_":"0x3","attr3":""}]}`, b.String())
}
