/*
 * Copyright 2023 Dgraph Labs, Inc. and Contributors
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

package dgraphtest

import (
	"testing"
	"time"
)

func TestCluster(t *testing.T) {
	c, err := NewCluster(NewClusterConfig().WithNumZeros(1).WithNumAlphas(1).WithACL("20s"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Cleanup()

	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Second)
	if err := c.Stop(); err != nil {
		t.Fatal(err)
	}
}
