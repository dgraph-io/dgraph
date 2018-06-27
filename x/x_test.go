/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package x

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRemoveDuplicates(t *testing.T) {
	set := RemoveDuplicates([]string{"a", "a", "a", "b", "b", "c", "c"})
	require.EqualValues(t, []string{"a", "b", "c"}, set)
}

func TestRemoveDuplicatesWithoutDuplicates(t *testing.T) {
	set := RemoveDuplicates([]string{"a", "b", "c", "d"})
	require.EqualValues(t, []string{"a", "b", "c", "d"}, set)
}
