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
	var wg sync.WaitGroup

	for x := 0; x < runtime.NumCPU(); x++ {
		n := makeFastJsonNode()
		require.NotNil(t, n)
		for i := 0; i < 30000; i++ {
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
