package worker

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDistance(t *testing.T) {
	require.Equal(t, 0, levenshteinDistance("detour", "detour"))
	require.Equal(t, 1, levenshteinDistance("detour", "det.our"))
	require.Equal(t, 2, levenshteinDistance("detour", "det..our"))
	require.Equal(t, 4, levenshteinDistance("detour", "..det..our"))
	require.Equal(t, 2, levenshteinDistance("detour", "detour.."))
	require.Equal(t, 3, levenshteinDistance("detour", "detour..."))
	require.Equal(t, 3, levenshteinDistance("detour", "...detour"))
	require.Equal(t, 3, levenshteinDistance("detour", "..detour."))
	require.Equal(t, 1, levenshteinDistance("detour", "detoar"))
	require.Equal(t, 6, levenshteinDistance("detour", "DETOUR"))
}
