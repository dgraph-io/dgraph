/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package pb

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTabletIndex_SetGetDelete(t *testing.T) {
	ti := NewTabletIndex()
	require.Equal(t, 0, ti.Len())

	tab1 := &Tablet{Predicate: "name", GroupId: 1}
	ti.Set("name", "", tab1)
	require.Equal(t, 1, ti.Len())

	got := ti.Get("name", "")
	require.Equal(t, tab1, got)

	// Different label is a different entry.
	tab2 := &Tablet{Predicate: "name", Label: "secret", GroupId: 2}
	ti.Set("name", "secret", tab2)
	require.Equal(t, 2, ti.Len())

	got = ti.Get("name", "secret")
	require.Equal(t, tab2, got)

	// Original is still there.
	got = ti.Get("name", "")
	require.Equal(t, tab1, got)

	// Miss returns nil.
	require.Nil(t, ti.Get("name", "other"))
	require.Nil(t, ti.Get("missing", ""))

	// Delete labeled.
	ti.Delete("name", "secret")
	require.Equal(t, 1, ti.Len())
	require.Nil(t, ti.Get("name", "secret"))

	// Delete unlabeled removes the predicate entry entirely.
	ti.Delete("name", "")
	require.Equal(t, 0, ti.Len())
	require.False(t, ti.HasPredicate("name"))

	// Delete of non-existent is no-op.
	ti.Delete("name", "")
}

func TestTabletIndex_GetAny_PrefersUnlabeled(t *testing.T) {
	ti := NewTabletIndex()

	tabUnlabeled := &Tablet{Predicate: "name", GroupId: 1}
	tabSecret := &Tablet{Predicate: "name", Label: "secret", GroupId: 2}
	tabTop := &Tablet{Predicate: "name", Label: "top_secret", GroupId: 3}

	ti.Set("name", "", tabUnlabeled)
	ti.Set("name", "secret", tabSecret)
	ti.Set("name", "top_secret", tabTop)

	// GetAny should prefer the unlabeled tablet.
	got := ti.GetAny("name")
	require.Equal(t, tabUnlabeled, got)

	// After deleting unlabeled, GetAny returns one of the labeled tablets.
	ti.Delete("name", "")
	got = ti.GetAny("name")
	require.NotNil(t, got)
	require.NotEmpty(t, got.Label)

	// Missing predicate returns nil.
	require.Nil(t, ti.GetAny("missing"))
}

func TestTabletIndex_GetForGroup(t *testing.T) {
	ti := NewTabletIndex()

	tab1 := &Tablet{Predicate: "name", GroupId: 1}
	tab2 := &Tablet{Predicate: "name", Label: "secret", GroupId: 2}
	tab3 := &Tablet{Predicate: "name", Label: "top_secret", GroupId: 3}

	ti.Set("name", "", tab1)
	ti.Set("name", "secret", tab2)
	ti.Set("name", "top_secret", tab3)

	// Exact group match.
	require.Equal(t, tab1, ti.GetForGroup("name", 1))
	require.Equal(t, tab2, ti.GetForGroup("name", 2))
	require.Equal(t, tab3, ti.GetForGroup("name", 3))

	// Non-existent group falls back to any tablet.
	got := ti.GetForGroup("name", 99)
	require.NotNil(t, got)

	// Missing predicate returns nil.
	require.Nil(t, ti.GetForGroup("missing", 1))
}

func TestTabletIndex_AllForPredicate(t *testing.T) {
	ti := NewTabletIndex()

	tab1 := &Tablet{Predicate: "name", GroupId: 1}
	tab2 := &Tablet{Predicate: "name", Label: "secret", GroupId: 2}
	ti.Set("name", "", tab1)
	ti.Set("name", "secret", tab2)

	labels := ti.AllForPredicate("name")
	require.Len(t, labels, 2)
	require.Equal(t, tab1, labels[""])
	require.Equal(t, tab2, labels["secret"])

	// Missing predicate returns nil.
	require.Nil(t, ti.AllForPredicate("missing"))
}

func TestTabletIndex_BuildFromFlat(t *testing.T) {
	flat := map[string]*Tablet{
		"name":            {Predicate: "name", GroupId: 1},
		"name@secret":     {Predicate: "name", Label: "secret", GroupId: 2},
		"name@top_secret": {Predicate: "name", Label: "top_secret", GroupId: 3},
		"age":             {Predicate: "age", GroupId: 1},
	}

	ti := NewTabletIndex()
	ti.BuildFromFlat(flat)
	require.Equal(t, 4, ti.Len())

	// Verify nested structure.
	require.True(t, ti.HasPredicate("name"))
	require.True(t, ti.HasPredicate("age"))

	nameLabels := ti.AllForPredicate("name")
	require.Len(t, nameLabels, 3)
	require.Equal(t, uint32(1), nameLabels[""].GroupId)
	require.Equal(t, uint32(2), nameLabels["secret"].GroupId)
	require.Equal(t, uint32(3), nameLabels["top_secret"].GroupId)

	ageLabels := ti.AllForPredicate("age")
	require.Len(t, ageLabels, 1)
	require.Equal(t, uint32(1), ageLabels[""].GroupId)
}

func TestTabletIndex_Range(t *testing.T) {
	ti := NewTabletIndex()
	ti.Set("name", "", &Tablet{Predicate: "name", GroupId: 1})
	ti.Set("name", "secret", &Tablet{Predicate: "name", Label: "secret", GroupId: 2})
	ti.Set("age", "", &Tablet{Predicate: "age", GroupId: 1})

	// Collect all entries.
	type entry struct{ pred, label string }
	var entries []entry
	ti.Range(func(pred, label string, tablet *Tablet) bool {
		entries = append(entries, entry{pred, label})
		return true
	})
	require.Len(t, entries, 3)

	// Test early termination.
	count := 0
	ti.Range(func(pred, label string, tablet *Tablet) bool {
		count++
		return false // stop after first
	})
	require.Equal(t, 1, count)
}

func TestTabletIndex_BuildFromFlat_MultipleGroups(t *testing.T) {
	// Simulate multiple groups each having tablets for the same predicate.
	ti := NewTabletIndex()

	group1 := map[string]*Tablet{
		"name": {Predicate: "name", GroupId: 1},
	}
	group2 := map[string]*Tablet{
		"name@secret": {Predicate: "name", Label: "secret", GroupId: 2},
	}
	group3 := map[string]*Tablet{
		"name@top_secret": {Predicate: "name", Label: "top_secret", GroupId: 3},
	}

	ti.BuildFromFlat(group1)
	ti.BuildFromFlat(group2)
	ti.BuildFromFlat(group3)

	require.Equal(t, 3, ti.Len())

	// GetForGroup should find each group's tablet.
	require.Equal(t, uint32(1), ti.GetForGroup("name", 1).GroupId)
	require.Equal(t, uint32(2), ti.GetForGroup("name", 2).GroupId)
	require.Equal(t, uint32(3), ti.GetForGroup("name", 3).GroupId)

	// GetAny should prefer unlabeled.
	require.Equal(t, uint32(1), ti.GetAny("name").GroupId)
}
