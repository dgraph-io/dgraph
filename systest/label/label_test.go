//go:build integration

/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgo/v250"
	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/dgraph-io/dgraph/v25/testutil"
)

// waitForCluster waits for all alpha nodes to be ready and connected to Zero.
func waitForCluster(t *testing.T) *dgo.Dgraph {
	t.Log("Waiting for cluster to be ready...")
	var dg *dgo.Dgraph
	var err error

	// Retry getting connection to group 1 (unlabeled alpha)
	t.Log("  Connecting to group 1 (unlabeled alpha)...")
	for i := 0; i < 30; i++ {
		dg, err = testutil.GetClientToGroup("1")
		if err == nil {
			t.Log("  Connected to group 1")
			break
		}
		if i%5 == 0 {
			t.Logf("  Retry %d/30: %v", i+1, err)
		}
		time.Sleep(time.Second)
	}
	require.NoError(t, err, "error while getting connection to group 1")

	// Wait for all groups to appear in state
	t.Log("  Waiting for all 3 groups to appear in state...")
	for i := 0; i < 30; i++ {
		state, err := testutil.GetState()
		if err == nil && len(state.Groups) >= 3 {
			t.Logf("  All %d groups ready", len(state.Groups))
			return dg
		}
		if i%5 == 0 {
			groupCount := 0
			if state != nil {
				groupCount = len(state.Groups)
			}
			t.Logf("  Retry %d/30: %d groups found", i+1, groupCount)
		}
		time.Sleep(time.Second)
	}

	t.Fatal("timeout waiting for all 3 groups to be ready")
	return nil
}

// TestLabeledAlphaRegistration verifies that alphas register with their labels.
func TestLabeledAlphaRegistration(t *testing.T) {
	t.Log("=== TestLabeledAlphaRegistration: Verifying alphas register with their labels ===")
	waitForCluster(t)

	t.Log("Fetching cluster state...")
	state, err := testutil.GetState()
	require.NoError(t, err)

	// Verify we have 3 groups
	t.Logf("Found %d groups", len(state.Groups))
	require.Len(t, state.Groups, 3, "expected 3 groups")

	// Track which labels we found
	foundLabels := make(map[string]uint32) // label -> groupId

	for groupID, group := range state.Groups {
		for _, member := range group.Members {
			if member.Label != "" {
				foundLabels[member.Label] = uint32(member.GroupID)
				t.Logf("Group %s has member with label: %s", groupID, member.Label)
			} else {
				t.Logf("Group %s has unlabeled member", groupID)
			}
		}
	}

	// Verify we have the expected labels
	require.Contains(t, foundLabels, "secret", "expected 'secret' label to be registered")
	require.Contains(t, foundLabels, "top_secret", "expected 'top_secret' label to be registered")
}

// TestLabeledPredicateRouting verifies that predicates with @label are routed to the correct group.
func TestLabeledPredicateRouting(t *testing.T) {
	t.Log("=== TestLabeledPredicateRouting: Verifying @label predicates route to correct groups ===")
	dg := waitForCluster(t)
	ctx := context.Background()

	// Drop all data first
	t.Log("Dropping all data...")
	require.NoError(t, dg.Alter(ctx, &api.Operation{DropAll: true}))
	t.Log("Drop complete")

	// Apply schema with labeled predicates
	t.Log("Applying schema with labeled predicates:")
	t.Log("  - name: unlabeled")
	t.Log("  - codename: @label(secret)")
	t.Log("  - alias: @label(top_secret)")
	schema := `
		name: string @index(term) .
		codename: string @index(term) @label(secret) .
		alias: string @index(term) @label(top_secret) .
	`
	require.NoError(t, dg.Alter(ctx, &api.Operation{Schema: schema}))
	t.Log("Schema applied successfully")

	// Give Zero time to process tablet assignments
	t.Log("Waiting 3s for Zero to process tablet assignments...")
	time.Sleep(3 * time.Second)

	// Get state and verify tablet assignments
	t.Log("Fetching cluster state to verify tablet assignments...")
	state, err := testutil.GetState()
	require.NoError(t, err)

	// Build a map of label -> groupID from members
	labelToGroup := make(map[string]string)
	for groupID, group := range state.Groups {
		for _, member := range group.Members {
			if member.Label != "" {
				labelToGroup[member.Label] = groupID
			}
		}
	}

	// Find which group has each predicate
	predicateToGroup := make(map[string]string)
	predicateToLabel := make(map[string]string)
	for groupID, group := range state.Groups {
		for predName, tablet := range group.Tablets {
			predicateToGroup[predName] = groupID
			predicateToLabel[predName] = tablet.Label
			t.Logf("Predicate %s is in group %s (tablet label: %s)", predName, groupID, tablet.Label)
		}
	}

	// Predicates are namespaced with "0-" prefix (namespace 0 is default)
	// Verify 'name' is in an unlabeled group (group 1)
	t.Log("Verifying predicate routing:")
	t.Logf("  Checking 'name' is in group 1 (unlabeled)... actual: %s", predicateToGroup["0-name"])
	require.Equal(t, "1", predicateToGroup["0-name"],
		"'name' predicate should be in group 1 (unlabeled)")

	// Verify 'codename' is in the 'secret' labeled group
	secretGroup := labelToGroup["secret"]
	t.Logf("  'secret' label maps to group: %s", secretGroup)
	require.NotEmpty(t, secretGroup, "should have a 'secret' labeled group")
	t.Logf("  Checking 'codename' is in secret group... actual: %s", predicateToGroup["0-codename"])
	require.Equal(t, secretGroup, predicateToGroup["0-codename"],
		"'codename' predicate should be in the 'secret' labeled group")

	// Verify 'alias' is in the 'top_secret' labeled group
	topSecretGroup := labelToGroup["top_secret"]
	t.Logf("  'top_secret' label maps to group: %s", topSecretGroup)
	require.NotEmpty(t, topSecretGroup, "should have a 'top_secret' labeled group")
	t.Logf("  Checking 'alias' is in top_secret group... actual: %s", predicateToGroup["0-alias"])
	require.Equal(t, topSecretGroup, predicateToGroup["0-alias"],
		"'alias' predicate should be in the 'top_secret' labeled group")
	t.Log("All predicate routing verified successfully!")
}

// TestLabeledPredicateDataIsolation verifies data can be written/read for labeled predicates.
func TestLabeledPredicateDataIsolation(t *testing.T) {
	t.Log("=== TestLabeledPredicateDataIsolation: Verifying data read/write across labeled predicates ===")
	dg := waitForCluster(t)
	ctx := context.Background()

	// Drop all and set up schema
	t.Log("Dropping all data...")
	require.NoError(t, dg.Alter(ctx, &api.Operation{DropAll: true}))

	t.Log("Applying schema with labeled predicates...")
	schema := `
		name: string @index(term) .
		codename: string @index(term) @label(secret) .
		alias: string @index(term) @label(top_secret) .
	`
	require.NoError(t, dg.Alter(ctx, &api.Operation{Schema: schema}))
	t.Log("Waiting 2s for schema to propagate...")
	time.Sleep(2 * time.Second)

	// Insert data using all predicates
	t.Log("Inserting test data (James Bond agent)...")
	_, err := dg.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
			_:agent <name> "James Bond" .
			_:agent <codename> "007" .
			_:agent <alias> "The spy who loved me" .
		`),
	})
	require.NoError(t, err)
	t.Log("Data inserted successfully")

	// Query to verify data is accessible
	t.Log("Querying data to verify accessibility across groups...")
	resp, err := dg.NewTxn().Query(ctx, `
		{
			agents(func: has(name)) {
				name
				codename
				alias
			}
		}
	`)
	require.NoError(t, err)
	t.Logf("Query response: %s", string(resp.GetJson()))

	// Verify the response contains our data
	t.Log("Verifying response matches expected data...")
	testutil.CompareJSON(t, `
		{
			"agents": [
				{
					"name": "James Bond",
					"codename": "007",
					"alias": "The spy who loved me"
				}
			]
		}
	`, string(resp.GetJson()))
	t.Log("Data isolation test passed!")
}

// TestLabeledPredicateCannotBeMoved verifies that labeled predicates cannot be moved via /moveTablet.
func TestLabeledPredicateCannotBeMoved(t *testing.T) {
	t.Log("=== TestLabeledPredicateCannotBeMoved: Verifying labeled predicates cannot be moved ===")
	dg := waitForCluster(t)
	ctx := context.Background()

	// Set up schema with labeled predicate
	t.Log("Dropping all data...")
	require.NoError(t, dg.Alter(ctx, &api.Operation{DropAll: true}))

	t.Log("Applying schema with 'codename' @label(secret)...")
	schema := `
		name: string @index(term) .
		codename: string @index(term) @label(secret) .
	`
	require.NoError(t, dg.Alter(ctx, &api.Operation{Schema: schema}))
	t.Log("Waiting 2s for schema to propagate...")
	time.Sleep(2 * time.Second)

	// Get state to find the 'secret' group
	t.Log("Fetching cluster state to find 'codename' predicate location...")
	state, err := testutil.GetState()
	require.NoError(t, err)

	// Find the group with 'codename' predicate (stored with namespace prefix "0-")
	var codenameGroup string
	for groupID, group := range state.Groups {
		if _, ok := group.Tablets["0-codename"]; ok {
			codenameGroup = groupID
			break
		}
	}
	t.Logf("'codename' predicate is currently in group: %s", codenameGroup)
	require.NotEmpty(t, codenameGroup, "codename predicate should exist")

	// Try to move 'codename' to group 1 (should fail)
	// Note: moveTablet API uses the namespaced predicate name
	moveUrl := fmt.Sprintf("http://"+testutil.GetSockAddrZeroHttp()+"/moveTablet?tablet=%s&group=1",
		url.QueryEscape("0-codename"))
	t.Logf("Attempting to move 'codename' to group 1 via: %s", moveUrl)
	resp, err := http.Get(moveUrl)
	require.NoError(t, err)
	defer resp.Body.Close()
	t.Logf("Move request returned status: %s", resp.Status)

	// The move should fail because labeled predicates cannot be moved
	// We expect either an error response or the predicate to remain in its original group
	t.Log("Waiting 2s for any potential move to complete...")
	time.Sleep(2 * time.Second)

	// Verify the predicate is still in its original group
	t.Log("Verifying predicate did not move...")
	state2, err := testutil.GetState()
	require.NoError(t, err)

	var newCodenameGroup string
	for groupID, group := range state2.Groups {
		if _, ok := group.Tablets["0-codename"]; ok {
			newCodenameGroup = groupID
			break
		}
	}
	t.Logf("'codename' predicate is now in group: %s", newCodenameGroup)

	require.Equal(t, codenameGroup, newCodenameGroup,
		"labeled predicate 'codename' should not have moved")
	t.Log("Verified: labeled predicate cannot be moved!")
}

// TestUnlabeledPredicateNotOnLabeledGroup verifies that unlabeled predicates
// are not assigned to labeled groups.
func TestUnlabeledPredicateNotOnLabeledGroup(t *testing.T) {
	t.Log("=== TestUnlabeledPredicateNotOnLabeledGroup: Verifying unlabeled predicates stay on unlabeled groups ===")
	dg := waitForCluster(t)
	ctx := context.Background()

	// Set up schema with mix of labeled and unlabeled predicates
	t.Log("Dropping all data...")
	require.NoError(t, dg.Alter(ctx, &api.Operation{DropAll: true}))

	t.Log("Applying schema with mix of labeled and unlabeled predicates...")
	schema := `
		name: string @index(term) .
		email: string @index(exact) .
		phone: string .
		codename: string @index(term) @label(secret) .
	`
	require.NoError(t, dg.Alter(ctx, &api.Operation{Schema: schema}))
	t.Log("Waiting 3s for schema to propagate...")
	time.Sleep(3 * time.Second)

	// Get state
	t.Log("Fetching cluster state...")
	state, err := testutil.GetState()
	require.NoError(t, err)

	// Find labeled groups
	t.Log("Identifying labeled groups...")
	labeledGroups := make(map[string]bool)
	for groupID, group := range state.Groups {
		for _, member := range group.Members {
			if member.Label != "" {
				labeledGroups[groupID] = true
				t.Logf("  Group %s is labeled (label: %s)", groupID, member.Label)
			}
		}
	}

	// Verify unlabeled predicates are not in labeled groups
	t.Log("Verifying unlabeled predicates are not in labeled groups...")
	unlabeledPreds := []string{"name", "email", "phone"}
	for _, pred := range unlabeledPreds {
		for groupID, group := range state.Groups {
			if _, ok := group.Tablets[pred]; ok {
				t.Logf("  Predicate '%s' is in group %s (labeled: %v)", pred, groupID, labeledGroups[groupID])
				require.False(t, labeledGroups[groupID],
					"unlabeled predicate '%s' should not be in labeled group %s", pred, groupID)
			}
		}
	}
	t.Log("Verified: unlabeled predicates are not on labeled groups!")
}

// TestMissingLabelGroupError tests that applying a schema with a label that has no matching
// alpha group results in an error.
func TestMissingLabelGroupError(t *testing.T) {
	t.Log("=== TestMissingLabelGroupError: Verifying error when using non-existent label ===")
	dg := waitForCluster(t)
	ctx := context.Background()

	t.Log("Dropping all data...")
	require.NoError(t, dg.Alter(ctx, &api.Operation{DropAll: true}))

	// Try to apply schema with a label that doesn't exist
	t.Log("Attempting to apply schema with @label(nonexistent_label)...")
	schema := `
		secret_data: string @label(nonexistent_label) .
	`
	err := dg.Alter(ctx, &api.Operation{Schema: schema})

	// This should fail because there's no alpha with --label=nonexistent_label
	if err != nil {
		t.Logf("Got expected error: %v", err)
	} else {
		t.Log("WARNING: No error returned (expected an error)")
	}
	require.Error(t, err, "should error when no alpha has the required label")
	require.Contains(t, err.Error(), "nonexistent_label",
		"error should mention the missing label")
	t.Log("Verified: non-existent label produces correct error!")
}
