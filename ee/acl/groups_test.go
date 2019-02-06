// +build !oss

/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
 */
package acl

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpdateAcl(t *testing.T) {
	var currenAcls []Acl
	newAcl := Acl{
		Predicate: "friend",
		Perm:      4,
	}

	updatedAcls1, changed := updateAcl(currenAcls, newAcl)
	require.True(t, changed, "the acl list should be changed")
	require.Equal(t, 1, len(updatedAcls1),
		"the updated acl list should have 1 element")

	// trying to update the acl list again with the exactly same acl won't change it
	updatedAcls2, changed := updateAcl(updatedAcls1, newAcl)
	require.False(t, changed,
		"the acl list should not be changed through update with an existing element")
	require.Equal(t, 1, len(updatedAcls2),
		"the updated acl list should still have 1 element")
	require.Equal(t, int32(4), updatedAcls2[0].Perm,
		"the perm should still have the value of 4")

	newAcl.Perm = 6
	updatedAcls3, changed := updateAcl(updatedAcls1, newAcl)
	require.True(t, changed, "the acl list should be changed through update "+
		"with element of new perm")
	require.Equal(t, 1, len(updatedAcls3),
		"the updated acl list should still have 1 element")
	require.Equal(t, int32(6), updatedAcls3[0].Perm,
		"the updated perm should be 6 now")

	newAcl = Acl{
		Predicate: "buddy",
		Perm:      6,
	}

	updatedAcls4, changed := updateAcl(updatedAcls3, newAcl)
	require.True(t, changed, "the acl should be changed through update "+
		"with element of new predicate")
	require.Equal(t, 2, len(updatedAcls4),
		"the acl list should have 2 elements now")

	newAcl = Acl{
		Predicate: "buddy",
		Perm:      -3,
	}

	updatedAcls5, changed := updateAcl(updatedAcls4, newAcl)
	require.True(t, changed, "the acl should be changed through update "+
		"with element of negative predicate")
	require.Equal(t, 1, len(updatedAcls5),
		"the acl list should have 1 element now")
	require.Equal(t, "friend", updatedAcls5[0].Predicate,
		"the left acl should have the original first predicate")
}
