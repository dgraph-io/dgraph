/*
 * Copyright 2016 DGraph Labs, Inc.
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

package schema

import (
	"testing"
)

func TestSchema(t *testing.T) {
	err := Parse("testfiles/test_schema")
	if err != nil {
		t.Error(err)
	}
}

func TestSchema1_Error(t *testing.T) {
	err := Parse("testfiles/test_schema1")
	if err == nil {
		t.Error("Expected error")
	}
}

func TestSchema2_Error(t *testing.T) {
	err := Parse("testfiles/test_schema2")
	if err == nil {
		t.Error("Expected error")
	}
}

func TestSchema3_Error(t *testing.T) {
	err := Parse("testfiles/test_schema3")
	if err == nil {
		t.Error("Expected error")
	}
}

func TestSchema4_Error(t *testing.T) {
	err := Parse("testfiles/test_schema4")
	if err.Error() == "Object type Actor with no fields" {
		t.Error("Expected error")
	}
}
