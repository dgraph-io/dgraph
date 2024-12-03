/*
 * Copyright 2024 Dgraph Labs, Inc. and Contributors
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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/v24/protos/pb"
)

func TestCompareSchemaUpdates(t *testing.T) {
	tests := []struct {
		name     string
		original *pb.SchemaUpdate
		update   *pb.SchemaUpdate
		expected []string
	}{
		{
			name: "No changes",
			original: &pb.SchemaUpdate{
				Predicate: "name",
				ValueType: pb.Posting_STRING,
				Count:     true,
			},
			update: &pb.SchemaUpdate{
				Predicate: "name",
				ValueType: pb.Posting_STRING,
				Count:     true,
			},
			expected: []string(nil),
		},
		{
			name: "Predicate changed",
			original: &pb.SchemaUpdate{
				Predicate: "name",
				ValueType: pb.Posting_STRING,
				Count:     true,
			},
			update: &pb.SchemaUpdate{
				Predicate: "age",
				ValueType: pb.Posting_STRING,
				Count:     true,
			},
			expected: []string{"Predicate"},
		},
		{
			name: "Multiple fields changed",
			original: &pb.SchemaUpdate{
				Predicate: "name",
				ValueType: pb.Posting_STRING,
				Count:     true,
			},
			update: &pb.SchemaUpdate{
				Predicate: "age",
				ValueType: pb.Posting_STRING,
				Count:     false,
			},
			expected: []string{"Predicate", "Count"},
		},
		{
			name: "Unchanged and changed fields",
			original: &pb.SchemaUpdate{
				Predicate: "name",
				Count:     true,
				Upsert:    false,
			},
			update: &pb.SchemaUpdate{
				Predicate: "name",
				Count:     true,
				Upsert:    true,
			},
			expected: []string{"Upsert"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, compareSchemaUpdates(tt.original, tt.update))
		})
	}
}
