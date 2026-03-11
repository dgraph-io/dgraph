//go:build integration2

/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/dgraph-io/dgraph/v25/dgraphtest"
	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestUniqueMultipleGroups(t *testing.T) {
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(2).WithNumZeros(1).WithReplicas(1)
	c, err := dgraphtest.NewLocalCluster(conf)
	require.NoError(t, err)
	defer func() { c.Cleanup(t.Failed()) }()
	require.NoError(t, c.Start())

	httpClient, err := c.HTTPClient()
	require.NoError(t, err)

	dg, cleanup, err := c.Client()
	require.NoError(t, err)
	defer cleanup()

	require.NoError(t, dg.DropAll())

	// Setup schema
	schema := `
        name: string @index(exact) .
        email_group_1: string @unique @index(exact) .
        email_group_2: string @unique @index(exact) .
    `
	require.NoError(t, dg.SetupSchema(schema))
	time.Sleep(2 * time.Second)

	// Get the cluster state to find which alphas belong to which groups
	var state pb.MembershipState

	getStateAndFindGroups := func() (uint32, uint32) {
		healthResp, err := httpClient.GetAlphaState()
		require.NoError(t, err)
		require.NoError(t, protojson.Unmarshal(healthResp, &state))
		t.Logf("Retrieved cluster state with %d groups", len(state.Groups))
		var e1, e2 uint32
		for groupID, group := range state.Groups {
			for predName := range group.Tablets {

				if predName == "email_group_1" || predName == "0-email_group_1" {
					t.Logf("predName: %s", predName)
					t.Logf("groupID: %d", groupID)
					e1 = groupID
				}
				if predName == "email_group_2" || predName == "0-email_group_2" {
					t.Logf("predName: %s", predName)
					t.Logf("groupID: %d", groupID)
					e2 = groupID
				}
			}
		}
		return e1, e2
	}

	email1GroupID, email2GroupID := getStateAndFindGroups()
	require.NotZero(t, email1GroupID, "email_group_1 predicate should be assigned to a group")
	require.NotZero(t, email2GroupID, "email_group_2 predicate should be assigned to a group")

	// If both predicates are in the same group, move one to a different group
	if email1GroupID == email2GroupID {
		var targetGroupID uint32
		for gid := range state.Groups {
			if gid != email1GroupID && gid > 0 {
				targetGroupID = gid
				break
			}
		}
		require.NotZero(t, targetGroupID, "need at least two groups to move predicate")
		err = httpClient.MoveTablet("email_group_1", targetGroupID)
		if err != nil {
			require.Contains(t, err.Error(), "already being served by group")
		}
		t.Logf("Moved email_group_1 predicate to group %d (was in same group as email_group_2)", targetGroupID)
		time.Sleep(2 * time.Second)
		email1GroupID, email2GroupID = getStateAndFindGroups()
	}

	require.NotEqual(t, email1GroupID, email2GroupID, "email_group_1 and email_group_2 should be in different groups")

	t.Logf("email_group_1 is in group %d, email_group_2 is in group %d", email1GroupID, email2GroupID)

	// Helper to find an alpha index in a given group
	findAlphaInGroup := func(gid uint32) int {
		if group, ok := state.Groups[gid]; ok {
			for _, member := range group.Members {
				addr := member.Addr
				if strings.HasPrefix(addr, "alpha") {
					alphaNumStr := strings.TrimPrefix(addr, "alpha")
					alphaNumStr = strings.Split(alphaNumStr, ":")[0]
					alphaNum, err := strconv.Atoi(alphaNumStr)
					if err == nil && alphaNum >= 0 {
						return alphaNum
					}
				}
			}
		}
		return -1
	}
	alphaForEmailGroup1 := findAlphaInGroup(email1GroupID)
	alphaForEmailGroup2 := findAlphaInGroup(email2GroupID)
	require.GreaterOrEqual(t, alphaForEmailGroup1, 0, "should find an alpha serving email_group_1")
	require.GreaterOrEqual(t, alphaForEmailGroup2, 0, "should find an alpha serving email_group_2")

	// Insert initial values for both predicates using default client
	_, err = dg.Mutate(&api.Mutation{
		SetNquads: []byte(`<0x100> <name> "User One" .
             <0x100> <email_group_1> "dup@example.com" .
             <0x100> <email_group_2> "other@example.com" .`),
		CommitNow: true,
	})
	require.NoError(t, err)

	// Try to insert duplicate into email1 from an alpha that does NOT serve email1 (i.e. from email2's group)
	dgNotEmail1, cleanup1, err := c.AlphaClient(alphaForEmailGroup2)
	require.NoError(t, err)
	defer cleanup1()
	rdfDupEmail1 := `<0x200> <name> "User Two" .
             <0x200> <email_group_1> "dup@example.com" .`
	_, err = dgNotEmail1.Mutate(&api.Mutation{
		SetNquads: []byte(rdfDupEmail1),
		CommitNow: true,
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "could not insert duplicate value")

	// Try to insert duplicate into email2 from an alpha that does NOT serve email2 (i.e. from email1's group)
	dgNotEmail2, cleanup2, err := c.AlphaClient(alphaForEmailGroup1)
	require.NoError(t, err)
	defer cleanup2()
	rdfDupEmail2 := `<0x300> <name> "User Three" .
             <0x300> <email_group_2> "other@example.com" .`
	_, err = dgNotEmail2.Mutate(&api.Mutation{
		SetNquads: []byte(rdfDupEmail2),
		CommitNow: true,
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "could not insert duplicate value")
}
