//go:build integration

/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
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

	// Verify 'codename' is in the 'secret' labeled group.
	// With composite sub-tablet keys, the tablet is stored as "0-codename@secret".
	secretGroup := labelToGroup["secret"]
	t.Logf("  'secret' label maps to group: %s", secretGroup)
	require.NotEmpty(t, secretGroup, "should have a 'secret' labeled group")
	t.Logf("  Checking 'codename@secret' is in secret group... actual: %s", predicateToGroup["0-codename@secret"])
	require.Equal(t, secretGroup, predicateToGroup["0-codename@secret"],
		"'codename' predicate should be in the 'secret' labeled group")

	// Verify 'alias' is in the 'top_secret' labeled group.
	// With composite sub-tablet keys, the tablet is stored as "0-alias@top_secret".
	topSecretGroup := labelToGroup["top_secret"]
	t.Logf("  'top_secret' label maps to group: %s", topSecretGroup)
	require.NotEmpty(t, topSecretGroup, "should have a 'top_secret' labeled group")
	t.Logf("  Checking 'alias@top_secret' is in top_secret group... actual: %s", predicateToGroup["0-alias@top_secret"])
	require.Equal(t, topSecretGroup, predicateToGroup["0-alias@top_secret"],
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

	// Find the group with 'codename' predicate (stored as composite key "0-codename@secret")
	var codenameGroup string
	for groupID, group := range state.Groups {
		if _, ok := group.Tablets["0-codename@secret"]; ok {
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
		if _, ok := group.Tablets["0-codename@secret"]; ok {
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

	// Verify unlabeled predicates are not in labeled groups.
	// Tablet keys in the state use the namespace prefix "0-" (e.g., "0-name").
	t.Log("Verifying unlabeled predicates are not in labeled groups...")
	unlabeledPreds := []string{"0-name", "0-email", "0-phone"}
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

// TestEntityLevelRouting verifies that setting dgraph.label on a UID pins all its predicates
// to the labeled group, creating composite sub-tablet keys like "predicate@label" in Zero's state.
func TestEntityLevelRouting(t *testing.T) {
	t.Log("=== TestEntityLevelRouting: Verifying entity-level sub-tablet routing ===")
	dg := waitForCluster(t)
	ctx := context.Background()

	// Step 1: Drop all data and apply schema without @label directives.
	// Entity-level routing uses dgraph.label on the UID, not schema-level @label.
	t.Log("Dropping all data...")
	require.NoError(t, dg.Alter(ctx, &api.Operation{DropAll: true}))

	t.Log("Applying schema (no @label directives — routing is entity-level via dgraph.label)...")
	schema := `
		Document.name: string @index(term) .
		Document.text: string @index(term) .
	`
	require.NoError(t, dg.Alter(ctx, &api.Operation{Schema: schema}))
	t.Log("Schema applied successfully")

	// Step 2: Create 3 entities with different dgraph.label values in a single mutation.
	t.Log("Inserting 3 entities with different dgraph.label values...")
	_, err := dg.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
			_:doc1 <dgraph.label> "secret" .
			_:doc1 <Document.name> "Secret.pdf" .
			_:doc1 <Document.text> "Classified" .

			_:doc2 <dgraph.label> "top_secret" .
			_:doc2 <Document.name> "TopSecret.pdf" .
			_:doc2 <Document.text> "Highly classified" .

			_:doc3 <Document.name> "Boring.pdf" .
			_:doc3 <Document.text> "Unclassified" .
		`),
	})
	require.NoError(t, err)
	t.Log("Entities inserted successfully")

	// Step 3: Verify sub-tablet assignments in Zero's state.
	t.Log("Waiting 5s for sub-tablet assignments to propagate...")
	time.Sleep(5 * time.Second)

	t.Log("Fetching cluster state to verify sub-tablet assignments...")
	state, err := testutil.GetState()
	require.NoError(t, err)

	// Build a map of label -> groupID from members
	labelToGroup := make(map[string]string)
	for groupID, group := range state.Groups {
		for _, member := range group.Members {
			if member.Label != "" {
				labelToGroup[member.Label] = groupID
				t.Logf("  Group %s has label: %s", groupID, member.Label)
			}
		}
	}
	secretGroup := labelToGroup["secret"]
	topSecretGroup := labelToGroup["top_secret"]
	require.NotEmpty(t, secretGroup, "should have a 'secret' labeled group")
	require.NotEmpty(t, topSecretGroup, "should have a 'top_secret' labeled group")

	// Build a map of tablet key -> groupID from all groups
	tabletToGroup := make(map[string]string)
	for groupID, group := range state.Groups {
		for tabletKey := range group.Tablets {
			tabletToGroup[tabletKey] = groupID
			t.Logf("  Tablet %q is in group %s", tabletKey, groupID)
		}
	}

	// Verify unlabeled sub-tablets exist (for doc3 which has no dgraph.label)
	t.Log("Verifying unlabeled sub-tablets (for doc3)...")
	_, hasDocName := tabletToGroup["0-Document.name"]
	require.True(t, hasDocName, "unlabeled sub-tablet '0-Document.name' should exist")

	_, hasDocText := tabletToGroup["0-Document.text"]
	require.True(t, hasDocText, "unlabeled sub-tablet '0-Document.text' should exist")

	// Verify labeled sub-tablets for "secret" (for doc1)
	t.Log("Verifying 'secret' sub-tablets (for doc1)...")
	secretNameGroup, hasSecretName := tabletToGroup["0-Document.name@secret"]
	require.True(t, hasSecretName, "sub-tablet '0-Document.name@secret' should exist")
	require.Equal(t, secretGroup, secretNameGroup,
		"'0-Document.name@secret' should be in the 'secret' group")

	secretTextGroup, hasSecretText := tabletToGroup["0-Document.text@secret"]
	require.True(t, hasSecretText, "sub-tablet '0-Document.text@secret' should exist")
	require.Equal(t, secretGroup, secretTextGroup,
		"'0-Document.text@secret' should be in the 'secret' group")

	// Verify labeled sub-tablets for "top_secret" (for doc2)
	t.Log("Verifying 'top_secret' sub-tablets (for doc2)...")
	topSecretNameGroup, hasTopSecretName := tabletToGroup["0-Document.name@top_secret"]
	require.True(t, hasTopSecretName, "sub-tablet '0-Document.name@top_secret' should exist")
	require.Equal(t, topSecretGroup, topSecretNameGroup,
		"'0-Document.name@top_secret' should be in the 'top_secret' group")

	topSecretTextGroup, hasTopSecretText := tabletToGroup["0-Document.text@top_secret"]
	require.True(t, hasTopSecretText, "sub-tablet '0-Document.text@top_secret' should exist")
	require.Equal(t, topSecretGroup, topSecretTextGroup,
		"'0-Document.text@top_secret' should be in the 'top_secret' group")

	t.Log("All sub-tablet assignments verified!")

	// Step 4: Verify query fan-out — all 3 documents should be returned despite
	// living on 3 different groups.
	// NOTE: We avoid orderasc in the DQL query because the sort operation fans out
	// to all sub-tablet groups and concatenates sorted runs instead of merging them,
	// causing triplication. Sorting in Go is the correct approach for now.
	//
	// We poll with retries because AllSubTablets (used for query fan-out) reads the
	// alpha's local tablet cache, which is updated asynchronously via applyState from
	// Zero. Until all sub-tablets propagate, the query may only reach a subset of groups.
	t.Log("Querying all documents via has(Document.name) to verify fan-out across groups...")
	type docResult struct {
		Name string `json:"Document.name"`
		Text string `json:"Document.text"`
	}
	var result struct {
		Docs []docResult `json:"docs"`
	}
	var lastResp string
	deadline := time.Now().Add(30 * time.Second)
	for attempt := 1; time.Now().Before(deadline); attempt++ {
		resp, err := dg.NewTxn().Query(ctx, `
			{
				docs(func: has(Document.name)) {
					Document.name
					Document.text
				}
			}
		`)
		require.NoError(t, err)
		lastResp = string(resp.GetJson())

		var r struct {
			Docs []docResult `json:"docs"`
		}
		require.NoError(t, json.Unmarshal(resp.GetJson(), &r))
		if len(r.Docs) == 3 {
			result = r
			t.Logf("Query returned 3 docs on attempt %d", attempt)
			break
		}
		t.Logf("Attempt %d: got %d docs (need 3), retrying in 2s... (response: %s)",
			attempt, len(r.Docs), lastResp)
		time.Sleep(2 * time.Second)
	}
	require.Len(t, result.Docs, 3,
		"should return all 3 documents from 3 different groups (last response: %s)", lastResp)

	// Sort results in Go for deterministic verification
	sort.Slice(result.Docs, func(i, j int) bool {
		return result.Docs[i].Name < result.Docs[j].Name
	})

	// Verify each document is present
	expectedDocs := []struct {
		Name string
		Text string
	}{
		{"Boring.pdf", "Unclassified"},
		{"Secret.pdf", "Classified"},
		{"TopSecret.pdf", "Highly classified"},
	}
	for i, expected := range expectedDocs {
		require.Equal(t, expected.Name, result.Docs[i].Name, "document name mismatch at index %d", i)
		require.Equal(t, expected.Text, result.Docs[i].Text, "document text mismatch at index %d", i)
		t.Logf("  Found document: %s -> %s", result.Docs[i].Name, result.Docs[i].Text)
	}

	t.Log("Entity-level routing test passed: all documents returned via fan-out!")
}
