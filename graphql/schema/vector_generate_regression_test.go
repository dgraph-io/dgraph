/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package schema

import (
	"fmt"
	"strings"
	"testing"

	"github.com/dgraph-io/dgraph/v25/x"
)

// TestVectorGenerateRestrictions keeps vector query generation independent of mutation generation.
func TestVectorGenerateRestrictions(t *testing.T) {
	for _, add := range []bool{false, true} {
		for _, update := range []bool{false, true} {
			for _, remove := range []bool{false, true} {
				t.Run(fmt.Sprintf("add=%t/update=%t/delete=%t", add, update, remove), func(t *testing.T) {
					input := fmt.Sprintf(`type VectorSchemaProbe
						@generate(mutation: {add: %t, update: %t, delete: %t}) {
						id: ID!
						name: String @search(by: [term])
						embedding: [Float!] @embedding @search(by: ["hnsw(metric: cosine)"])
					}`, add, update, remove)
					handler, err := NewHandler(input, false)
					if err != nil {
						t.Fatal(err)
					}
					generated := handler.GQLSchema()
					if _, err := FromString(generated, x.RootNamespace); err != nil {
						t.Fatal(err)
					}
					if strings.Contains(generated, "HNSWSearchFilter") {
						t.Fatal("vector index generated an undefined ordinary field filter")
					}
					for _, field := range []string{
						"name: StringTermFilter",
						"has: [VectorSchemaProbeHasFilter]",
						"embedding: [Float!]",
					} {
						if !strings.Contains(generated, field) {
							t.Fatalf("missing generated field %s", field)
						}
					}
					for _, query := range []string{
						"getVectorSchemaProbe(",
						"querySimilarVectorSchemaProbeByEmbedding(",
						"querySimilarVectorSchemaProbeById(",
					} {
						if !strings.Contains(generated, query) {
							t.Fatalf("missing generated query %s", query)
						}
					}
					for mutation, enabled := range map[string]bool{"add": add, "update": update, "delete": remove} {
						if strings.Contains(generated, mutation+"VectorSchemaProbe(") != enabled {
							t.Fatalf("mutation %s generation changed", mutation)
						}
					}
					const vectorPredicate = `VectorSchemaProbe.embedding: float32vector @index(hnsw(metric: "cosine"))`
					if !strings.Contains(handler.DGSchema(), vectorPredicate) {
						t.Fatalf("cosine index changed: %s", handler.DGSchema())
					}
				})
			}
		}
	}
}
