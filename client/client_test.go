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

	"github.com/dgraph-io/dgraph/protos/graphp"
	"github.com/stretchr/testify/assert"
)

func graphValue(x string) *graphp.Value {
	return &graphp.Value{&graphp.Value_StrVal{x}}
}

func TestCheckNQuad(t *testing.T) {
	s := graphValue("Alice")
	if err := checkNQuad(graphp.NQuad{
		Predicate:   "name",
		ObjectValue: s,
	}); err == nil {
		t.Fatal(err)
	}
	if err := checkNQuad(graphp.NQuad{
		Subject:     "alice",
		ObjectValue: s,
	}); err == nil {
		t.Fatal(err)
	}
	if err := checkNQuad(graphp.NQuad{
		Subject:   "alice",
		Predicate: "name",
	}); err == nil {
		t.Fatal(err)
	}
	if err := checkNQuad(graphp.NQuad{
		Subject:     "alice",
		Predicate:   "name",
		ObjectValue: s,
		ObjectId:    "id",
	}); err == nil {
		t.Fatal(err)
	}
}

func TestSetMutation(t *testing.T) {
	req := Req{}

	s := graphValue("Alice")
	if err := req.AddMutation(graphp.NQuad{
		Subject:     "alice",
		Predicate:   "name",
		ObjectValue: s,
	}, SET); err != nil {
		t.Fatal(err)
	}

	s = graphValue("rabbithole")
	if err := req.AddMutation(graphp.NQuad{
		Subject:     "alice",
		Predicate:   "falls.in",
		ObjectValue: s,
	}, SET); err != nil {
		t.Fatal(err)
	}

	if err := req.AddMutation(graphp.NQuad{
		Subject:     "alice",
		Predicate:   "falls.in",
		ObjectValue: s,
	}, DEL); err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, len(req.gr.Mutation.Set), 2, "Set should have 2 entries")
	assert.Equal(t, len(req.gr.Mutation.Del), 1, "Del should have 1 entry")
}

func TestAddSchema(t *testing.T) {
	req := Req{}

	if err := req.addSchema(graphp.SchemaUpdate{Predicate: "name"}); err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, len(req.gr.Mutation.Schema), 1)
}
