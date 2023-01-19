/*
 * Copyright 2017-2022 Dgraph Labs, Inc. and Contributors
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
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
)

func TestEncodeMemory(t *testing.T) {
	//	if testing.Short() {
	t.Skip("Skipping TestEncodeMemory")
	//	}
	var wg sync.WaitGroup

	for i := 0; i < runtime.NumCPU(); i++ {
		enc := newEncoder()
		n := enc.newNode(0)
		require.NotNil(t, n)
		for i := 0; i < 15000; i++ {
			enc.AddValue(n, enc.idForAttr(fmt.Sprintf("very long attr name %06d", i)),
				types.ValueForType(types.StringID))
			enc.AddListChild(n,
				enc.newNode(enc.idForAttr(fmt.Sprintf("another long child %06d", i))))
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				enc.buf.Reset()
				enc.encode(n)
			}
		}()
	}

	wg.Wait()
}

func TestNormalizeJSONLimit(t *testing.T) {
	// Set default normalize limit.
	x.Config.Limit = z.NewSuperFlag("normalize-node=10000;").MergeAndCheckDefault(worker.LimitDefaults)

	if testing.Short() {
		t.Skip("Skipping TestNormalizeJSONLimit")
	}

	enc := newEncoder()
	n := enc.newNode(enc.idForAttr("root"))
	require.NotNil(t, n)
	for i := 0; i < 1000; i++ {
		enc.AddValue(n, enc.idForAttr(fmt.Sprintf("very long attr name %06d", i)),
			types.ValueForType(types.StringID))
		child1 := enc.newNode(enc.idForAttr("child1"))
		enc.AddListChild(n, child1)
		for j := 0; j < 100; j++ {
			enc.AddValue(child1, enc.idForAttr(fmt.Sprintf("long child1 attr %06d", j)),
				types.ValueForType(types.StringID))
		}
		child2 := enc.newNode(enc.idForAttr("child2"))
		enc.AddListChild(n, child2)
		for j := 0; j < 100; j++ {
			enc.AddValue(child2, enc.idForAttr(fmt.Sprintf("long child2 attr %06d", j)),
				types.ValueForType(types.StringID))
		}
		child3 := enc.newNode(enc.idForAttr("child3"))
		enc.AddListChild(n, child3)
		for j := 0; j < 100; j++ {
			enc.AddValue(child3, enc.idForAttr(fmt.Sprintf("long child3 attr %06d", j)),
				types.ValueForType(types.StringID))
		}
	}
	_, err := enc.normalize(n)
	require.Error(t, err, "Couldn't evaluate @normalize directive - too many results")
}

func BenchmarkJsonMarshal(b *testing.B) {
	inputStrings := [][]string{
		[]string{"largestring", strings.Repeat("a", 1024)},
		[]string{"smallstring", "abcdef"},
		[]string{"specialchars", "<><>^)(*&(%*&%&^$*&%)(*&)^)"},
	}

	var result []byte

	for _, input := range inputStrings {
		b.Run(fmt.Sprintf("STDJsonMarshal-%s", input[0]), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				result, _ = json.Marshal(input[1])
			}
		})

		b.Run(fmt.Sprintf("stringJsonMarshal-%s", input[0]), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				result = stringJsonMarshal(input[1])
			}
		})
	}

	_ = result
}

func TestStringJsonMarshal(t *testing.T) {
	inputs := []string{
		"",
		"0",
		"true",
		"1.909045927350",
		"nil",
		"null",
		"<&>",
		`quoted"str"ing`,
	}

	for _, input := range inputs {
		gm, err := json.Marshal(input)
		require.NoError(t, err)

		sm := stringJsonMarshal(input)

		require.Equal(t, gm, sm)
	}
}

func TestFastJsonNode(t *testing.T) {
	attrId := uint16(20)
	scalarVal := bytes.Repeat([]byte("a"), 160)
	list := true

	enc := newEncoder()
	fj := enc.newNode(attrId)
	require.NoError(t, enc.setScalarVal(fj, scalarVal))
	enc.setList(fj, list)

	require.Equal(t, attrId, enc.getAttr(fj))
	sv, err := enc.getScalarVal(fj)
	require.NoError(t, err)
	require.Equal(t, scalarVal, sv)
	require.Equal(t, list, enc.getList(fj))

	fj2 := enc.newNode(attrId)
	require.NoError(t, enc.setScalarVal(fj2, scalarVal))
	enc.setList(fj2, list)

	sv, err = enc.getScalarVal(fj2)
	require.NoError(t, err)
	require.Equal(t, scalarVal, sv)
	require.Equal(t, list, enc.getList(fj2))

	enc.appendAttrs(fj, fj2)
	require.Equal(t, fj2, enc.children(fj))
}

func BenchmarkFastJsonNodeEmpty(b *testing.B) {
	for i := 0; i < b.N; i++ {
		enc := newEncoder()
		var fj fastJsonNode
		for i := 0; i < 2e6; i++ {
			fj = enc.newNode(0)
		}
		_ = fj
	}
}

var (
	testAttr = "abcdefghijklmnop"
	testVal  = types.Val{Tid: types.DefaultID, Value: []byte(testAttr)}
)

func buildTestTree(b *testing.B, enc *encoder, level, maxlevel int, fj fastJsonNode) {
	if level >= maxlevel {
		return
	}

	// Add only two children for now.
	for i := 0; i < 2; i++ {
		var ch fastJsonNode
		if level == maxlevel-1 {
			val, err := valToBytes(testVal)
			if err != nil {
				panic(err)
			}

			ch, err = enc.makeScalarNode(enc.idForAttr(testAttr), val, false)
			require.NoError(b, err)
		} else {
			ch := enc.newNode(enc.idForAttr(testAttr))
			buildTestTree(b, enc, level+1, maxlevel, ch)
		}
		enc.appendAttrs(fj, ch)
	}
}

func BenchmarkFastJsonNode2Chilren(b *testing.B) {
	for i := 0; i < b.N; i++ {
		enc := newEncoder()
		root := enc.newNode(enc.idForAttr(testAttr))
		buildTestTree(b, enc, 1, 20, root)
	}
}

func TestChildrenOrder(t *testing.T) {
	enc := newEncoder()
	root := enc.newNode(1)
	root.meta = 0
	for i := 1; i <= 10; i++ {
		n := enc.newNode(1)
		n.meta = uint64(i)
		enc.addChildren(root, n)
	}

	stepMom := enc.newNode(1)
	stepMom.meta = 100
	for i := 11; i <= 20; i++ {
		n := enc.newNode(1)
		n.meta = uint64(i)
		enc.addChildren(stepMom, n)
	}
	enc.addChildren(root, stepMom.child)

	stepDad := enc.newNode(1)
	stepDad.meta = 101
	{
		n := enc.newNode(1)
		n.meta = uint64(21)
		enc.addChildren(stepDad, n)
	}
	enc.addChildren(root, stepDad.child)

	enc.fixOrder(root)
	enc.fixOrder(root) // Another time just to ensure it still works.

	child := root.child
	for i := 1; i <= 21; i++ {
		require.Equal(t, uint64(i), child.meta&^visitedBit)
		child = child.next
	}
	require.Nil(t, child)
}
