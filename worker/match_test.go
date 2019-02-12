package worker

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDistance(t *testing.T) {
	require.Equal(t, 0, levenshteinDistance("detour", "detour", 2))
	require.Equal(t, 1, levenshteinDistance("detour", "det.our", 2))
	require.Equal(t, 2, levenshteinDistance("detour", "det..our", 2))
	require.Equal(t, 3, levenshteinDistance("detour", "..det..our", 2))
	require.Equal(t, 2, levenshteinDistance("detour", "detour..", 2))
	require.Equal(t, 3, levenshteinDistance("detour", "detour...", 2))
	require.Equal(t, 3, levenshteinDistance("detour", "...detour", 2))
	require.Equal(t, 3, levenshteinDistance("detour", "..detour.", 2))
}
