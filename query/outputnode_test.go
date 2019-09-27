/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package query

import (
	"bytes"
	"fmt"
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

func makeFastJsonNode() *fastJsonNode {
	return &fastJsonNode{}
}

func TestEncodeMemory(t *testing.T) {
	//	if testing.Short() {
	t.Skip("Skipping TestEncodeMemory")
	//	}
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
				var buf bytes.Buffer
				n.encode(&buf)
			}
		}()
	}

	wg.Wait()
}

func TestNormalizeJSONLimit(t *testing.T) {
	// Set default normalize limit.
	x.Config.NormalizeNodeLimit = 1e4

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
	require.Error(t, err, "Couldn't evaluate @normalize directive - too many results")
}

func BenchmarkNormalizePerformance(b *testing.B) {
	nodeHeight := 3
	treeDepth := 3

	b.Run("Old Normalize", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			root := (&fastJsonNode{}).New("_root_")
			fillNode(root.(*fastJsonNode), nodeHeight, treeDepth)

			b.StartTimer()
			_, _ = root.(*fastJsonNode).normalize()
			b.StopTimer()
		}
	})

	b.Run("New Normalize", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			root := (&fastJsonNode{}).New("_root_")
			rootChild := root.New("_rootchild")
			root.AddListChild("_rootchild_", rootChild)
			fillNode(rootChild.(*fastJsonNode), nodeHeight, treeDepth)
			rootChild.(*fastJsonNode).isNormalized = true

			b.StartTimer()
			_ = normalizeResult(rootChild.(*fastJsonNode), root.(*fastJsonNode), 0)
			b.StopTimer()
		}
	})

	fmt.Println("done")
}

func fillNode(node *fastJsonNode, nodeHeight, treeDepth int) {
	if treeDepth <= 0 {
		return
	}

	i := 0
	for ; i < nodeHeight/2; i++ {
		node.AddValue(fmt.Sprintf("Attribute %d", i), types.ValueForType(types.StringID))
	}

	for ; i < nodeHeight; i++ {
		child := node.New(fmt.Sprintf("Child%d", i))
		node.AddListChild(fmt.Sprintf("Child%d", i), child)
		node.AddValue(fmt.Sprintf("Attribute %d", i), types.ValueForType(types.StringID))
		fillNode(child.(*fastJsonNode), nodeHeight, treeDepth-1)
	}
}

func TestNormalizeJSONUid1(t *testing.T) {
	// Set default normalize limit.
	x.Config.NormalizeNodeLimit = 1e4

	n := (&fastJsonNode{}).New("root")
	require.NotNil(t, n)
	child1 := n.New("child1")
	child1.SetUID(uint64(1), "uid")
	child1.AddValue("attr1", types.ValueForType(types.StringID))
	n.AddListChild("child1", child1)

	child2 := n.New("child2")
	child2.SetUID(uint64(2), "uid")
	child2.AddValue("attr2", types.ValueForType(types.StringID))
	child1.AddListChild("child2", child2)

	child3 := n.New("child3")
	child3.SetUID(uint64(3), "uid")
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
	nn.(*fastJsonNode).encode(&b)
	require.JSONEq(t, `{"alias":[{"uid":"0x3","attr1":"","attr2":"","attr3":""}]}`, b.String())
}

func TestNormalizeJSONUid2(t *testing.T) {
	n := (&fastJsonNode{}).New("root")
	require.NotNil(t, n)
	child1 := n.New("child1")
	child1.SetUID(uint64(1), "uid")
	child1.AddValue("___attr1", types.ValueForType(types.StringID))
	n.AddListChild("child1", child1)

	child2 := n.New("child2")
	child2.SetUID(uint64(2), "uid")
	child2.AddValue("___attr2", types.ValueForType(types.StringID))
	child1.AddListChild("child2", child2)

	child3 := n.New("child3")
	child3.SetUID(uint64(3), "uid")
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
	nn.(*fastJsonNode).encode(&b)
	require.JSONEq(t, `{"alias":[{"___attr1":"","___attr2":"","uid":"0x3","attr3":""}]}`, b.String())
}
