/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors
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

package schema

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vektah/gqlparser/gqlerror"
)

func TestAddData_AddInitial(t *testing.T) {
	resp := &Response{}

	resp.AddData([]byte(`"Some": "Data"`))
	buf := new(bytes.Buffer)
	resp.WriteTo(buf)

	assert.JSONEq(t, `{"data": {"Some": "Data"}}`, buf.String())
}

func TestAddData_AddNothing(t *testing.T) {
	resp := &Response{}

	resp.AddData([]byte(`"Some": "Data"`))
	resp.AddData([]byte{})
	buf := new(bytes.Buffer)
	resp.WriteTo(buf)

	assert.JSONEq(t, `{"data": {"Some": "Data"}}`, buf.String())
}
func TestAddData_AddMore(t *testing.T) {
	resp := &Response{}

	resp.AddData([]byte(`"Some": "Data"`))
	resp.AddData([]byte(`"And": "More"`))
	buf := new(bytes.Buffer)
	resp.WriteTo(buf)

	assert.JSONEq(t, `{"data": {"Some": "Data", "And": "More"}}`, buf.String())
}

func TestWriteTo_ErrorsAndData(t *testing.T) {
	resp := &Response{Errors: gqlerror.List{gqlerror.Errorf("An Error")}}
	resp.AddData([]byte(`"Some": "Data"`))

	buf := new(bytes.Buffer)
	resp.WriteTo(buf)

	assert.JSONEq(t,
		`{"errors":[{"message":"An Error"}], "data": {"Some": "Data"}}`, buf.String())
}
