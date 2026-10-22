/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package schema

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The bm25 schema invariants must hold at PARSE level so every ingest path is
// covered (alter, live loader, and crucially the bulk loader, which never calls
// worker.ValidateSchema). The bulk stats fold dedupes per (attr, uid), so a list
// predicate slipping through would silently collapse distinct values' stats.
func TestParseRejectsBM25OnListPredicate(t *testing.T) {
	_, err := Parse(`pred: [string] @index(bm25) .`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot be applied to list predicate")
}

func TestParseRejectsBM25WithLang(t *testing.T) {
	_, err := Parse(`pred: string @lang @index(bm25) .`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "bm25 does not support language-qualified values")
}
