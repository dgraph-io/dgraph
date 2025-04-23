/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package schema

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hypermodeinc/dgraph/v25/protos/pb"
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
