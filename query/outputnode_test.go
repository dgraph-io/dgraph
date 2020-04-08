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
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
	"github.com/stretchr/testify/require"
)

func TestEncodeMemory(t *testing.T) {
	//	if testing.Short() {
	t.Skip("Skipping TestEncodeMemory")
	//	}
	var wg sync.WaitGroup

	for i := 0; i < runtime.NumCPU(); i++ {
		enc := newEncoder()
		n := enc.newFastJsonNode()
		require.NotNil(t, n)
		for i := 0; i < 15000; i++ {
			enc.AddValue(n, enc.idForAttr(fmt.Sprintf("very long attr name %06d", i)),
				types.ValueForType(types.StringID))
			enc.AddListChild(n, enc.idForAttr(fmt.Sprintf("another long child %06d", i)),
				enc.newFastJsonNode())
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				var buf bytes.Buffer
				enc.encode(n, &buf)
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

	enc := newEncoder()
	n := enc.newFastJsonNodeWithAttr(enc.idForAttr("root"))
	require.NotNil(t, n)
	for i := 0; i < 1000; i++ {
		enc.AddValue(n, enc.idForAttr(fmt.Sprintf("very long attr name %06d", i)),
			types.ValueForType(types.StringID))
		child1 := enc.newFastJsonNodeWithAttr(enc.idForAttr("child1"))
		enc.AddListChild(n, enc.idForAttr("child1"), child1)
		for j := 0; j < 100; j++ {
			enc.AddValue(child1, enc.idForAttr(fmt.Sprintf("long child1 attr %06d", j)),
				types.ValueForType(types.StringID))
		}
		child2 := enc.newFastJsonNodeWithAttr(enc.idForAttr("child2"))
		enc.AddListChild(n, enc.idForAttr("child2"), child2)
		for j := 0; j < 100; j++ {
			enc.AddValue(child2, enc.idForAttr(fmt.Sprintf("long child2 attr %06d", j)),
				types.ValueForType(types.StringID))
		}
		child3 := enc.newFastJsonNodeWithAttr(enc.idForAttr("child3"))
		enc.AddListChild(n, enc.idForAttr("child3"), child3)
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
	isChild := true
	list := true

	enc := newEncoder()
	fj := enc.newFastJsonNode()
	enc.setAttr(fj, attrId)
	enc.setScalarVal(fj, scalarVal)
	enc.setIsChild(fj, isChild)
	enc.setList(fj, list)

	require.Equal(t, attrId, enc.getAttr(fj))
	require.Equal(t, scalarVal, enc.getScalarVal(fj))
	require.Equal(t, isChild, enc.getIsChild(fj))
	require.Equal(t, list, enc.getList(fj))

	fj2 := enc.newFastJsonNode()
	enc.setAttr(fj2, attrId)
	enc.setScalarVal(fj2, scalarVal)
	enc.setIsChild(fj2, isChild)
	enc.setList(fj2, list)

	require.Equal(t, attrId, enc.getAttr(fj2))
	require.Equal(t, scalarVal, enc.getScalarVal(fj2))
	require.Equal(t, isChild, enc.getIsChild(fj2))
	require.Equal(t, list, enc.getList(fj2))
}

func BenchmarkFastJsonNodeMemory(b *testing.B) {
	for i := 0; i < b.N; i++ {
		enc := newEncoder()
		var fj fastJsonNode
		for i := 0; i < 2e6; i++ {
			fj = enc.newFastJsonNode()
		}
		_ = fj
	}
}
