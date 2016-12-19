/*
 * Copyright 2016 Dgraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckNQuad(t *testing.T) {
	if err := checkNQuad("", "name", "", Str("Alice")); err == nil {
		t.Fatal(err)
	}
	if err := checkNQuad("alice", "", "", Str("Alice")); err == nil {
		t.Fatal(err)
	}
	if err := checkNQuad("alice", "name", "", nil); err == nil {
		t.Fatal(err)
	}
	if err := checkNQuad("alice", "name", "id", Str("Alice")); err == nil {
		t.Fatal(err)
	}
}

func TestSetMutation(t *testing.T) {
	req := NewRequest()

	if err := req.SetMutation("alice", "name", "", Str("Alice"), ""); err != nil {
		t.Fatal(err)
	}
	if err := req.SetMutation("alice", "falls.in", "", Str("rabbithole"), ""); err != nil {
		t.Fatal(err)
	}
	if err := req.DelMutation("alice", "falls.in", "", Str("rabbithole"), ""); err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, len(req.gr.Mutation.Set), 2, "Set should have 2 entries")
	assert.Equal(t, len(req.gr.Mutation.Del), 1, "Del should have 1 entry")
}
