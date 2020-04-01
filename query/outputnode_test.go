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

	for i := 0; i < runtime.NumCPU(); i++ {
		n := makeFastJsonNode()
		require.NotNil(t, n)
		for i := 0; i < 15000; i++ {
			n.AddValue(fmt.Sprintf("very long attr name %06d", i), types.ValueForType(types.StringID))
			n.AddListChild(fmt.Sprintf("another long child %06d", i), &fastJsonNode{})
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
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
	_, err := n.normalize()
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
