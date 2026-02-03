//go:build integration

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/dgraph-io/dgraph/v25/x/auth"
)

// setupLabelAuthTest sets up the schema and test data for label auth tests.
// Returns the dgraph client for querying.
func setupLabelAuthTest(t *testing.T) {
	t.Log("Setting up label auth test...")
	dg := waitForCluster(t)
	ctx := context.Background()

	// Drop all and set up schema
	t.Log("Dropping all data...")
	require.NoError(t, dg.Alter(ctx, &api.Operation{DropAll: true}))

	// Schema with labeled predicates at different security levels
	t.Log("Applying schema with labeled predicates...")
	schema := `
		name: string @index(term) .
		codename: string @index(term) @label(secret) .
		alias: string @index(term) @label(top_secret) .
	`
	require.NoError(t, dg.Alter(ctx, &api.Operation{Schema: schema}))
	t.Log("Waiting 2s for schema to propagate...")
	time.Sleep(2 * time.Second)

	// Insert test data
	t.Log("Inserting test data...")
	_, err := dg.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
			_:agent <name> "James Bond" .
			_:agent <codename> "007" .
			_:agent <alias> "The spy who loved me" .
		`),
	})
	require.NoError(t, err)
	t.Log("Test data inserted successfully")
}

// TestLabelAuthNoLevel verifies that a user with no security level can only access unlabeled predicates.
func TestLabelAuthNoLevel(t *testing.T) {
	t.Log("=== TestLabelAuthNoLevel: User with no level should only see unlabeled predicates ===")
	setupLabelAuthTest(t)
	dg := waitForCluster(t)

	// Create context with auth credentials but empty level
	// This triggers label filtering (unlike nil token which skips filtering for backward compat)
	ctx := auth.AttachToOutgoingContext(context.Background(), &auth.AuthContext{Level: ""})

	// Query for all predicates
	t.Log("Querying with empty security level...")
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

	// Should only see 'name', not 'codename' or 'alias'
	var result struct {
		Agents []struct {
			Name     string `json:"name,omitempty"`
			Codename string `json:"codename,omitempty"`
			Alias    string `json:"alias,omitempty"`
		} `json:"agents"`
	}
	require.NoError(t, json.Unmarshal(resp.GetJson(), &result))

	require.Len(t, result.Agents, 1, "should have 1 agent")
	require.Equal(t, "James Bond", result.Agents[0].Name, "should see name")
	require.Empty(t, result.Agents[0].Codename, "should NOT see codename (requires secret)")
	require.Empty(t, result.Agents[0].Alias, "should NOT see alias (requires top_secret)")
	t.Log("Test passed: user with no level only sees unlabeled predicates")
}

// TestLabelAuthSecretLevel verifies that a user with 'secret' level can access secret and below.
func TestLabelAuthSecretLevel(t *testing.T) {
	t.Log("=== TestLabelAuthSecretLevel: User with 'secret' level should see name + codename ===")
	setupLabelAuthTest(t)
	dg := waitForCluster(t)

	// Create context with 'secret' security level via gRPC metadata
	ctx := auth.AttachToOutgoingContext(context.Background(), &auth.AuthContext{Level: "secret"})

	// Query for all predicates
	t.Log("Querying with 'secret' security level...")
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

	// Should see 'name' and 'codename', but not 'alias'
	var result struct {
		Agents []struct {
			Name     string `json:"name,omitempty"`
			Codename string `json:"codename,omitempty"`
			Alias    string `json:"alias,omitempty"`
		} `json:"agents"`
	}
	require.NoError(t, json.Unmarshal(resp.GetJson(), &result))

	require.Len(t, result.Agents, 1, "should have 1 agent")
	require.Equal(t, "James Bond", result.Agents[0].Name, "should see name")
	require.Equal(t, "007", result.Agents[0].Codename, "should see codename (has secret)")
	require.Empty(t, result.Agents[0].Alias, "should NOT see alias (requires top_secret)")
	t.Log("Test passed: user with 'secret' level sees name + codename")
}

// TestLabelAuthTopSecretLevel verifies that a user with 'top_secret' level can access all predicates.
func TestLabelAuthTopSecretLevel(t *testing.T) {
	t.Log("=== TestLabelAuthTopSecretLevel: User with 'top_secret' level should see all predicates ===")
	setupLabelAuthTest(t)
	dg := waitForCluster(t)

	// Create context with 'top_secret' security level via gRPC metadata
	ctx := auth.AttachToOutgoingContext(context.Background(), &auth.AuthContext{Level: "top_secret"})

	// Query for all predicates
	t.Log("Querying with 'top_secret' security level...")
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

	// Should see all predicates
	var result struct {
		Agents []struct {
			Name     string `json:"name,omitempty"`
			Codename string `json:"codename,omitempty"`
			Alias    string `json:"alias,omitempty"`
		} `json:"agents"`
	}
	require.NoError(t, json.Unmarshal(resp.GetJson(), &result))

	require.Len(t, result.Agents, 1, "should have 1 agent")
	require.Equal(t, "James Bond", result.Agents[0].Name, "should see name")
	require.Equal(t, "007", result.Agents[0].Codename, "should see codename")
	require.Equal(t, "The spy who loved me", result.Agents[0].Alias, "should see alias (has top_secret)")
	t.Log("Test passed: user with 'top_secret' level sees all predicates")
}

// TestLabelAuthUnclassifiedLevel verifies that 'unclassified' level can only access unlabeled predicates.
func TestLabelAuthUnclassifiedLevel(t *testing.T) {
	t.Log("=== TestLabelAuthUnclassifiedLevel: User with 'unclassified' level should only see unlabeled ===")
	setupLabelAuthTest(t)
	dg := waitForCluster(t)

	// Create context with 'unclassified' security level via gRPC metadata
	ctx := auth.AttachToOutgoingContext(context.Background(), &auth.AuthContext{Level: "unclassified"})

	// Query for all predicates
	t.Log("Querying with 'unclassified' security level...")
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

	// Should only see 'name' (unlabeled), not 'codename' (secret) or 'alias' (top_secret)
	var result struct {
		Agents []struct {
			Name     string `json:"name,omitempty"`
			Codename string `json:"codename,omitempty"`
			Alias    string `json:"alias,omitempty"`
		} `json:"agents"`
	}
	require.NoError(t, json.Unmarshal(resp.GetJson(), &result))

	require.Len(t, result.Agents, 1, "should have 1 agent")
	require.Equal(t, "James Bond", result.Agents[0].Name, "should see name")
	require.Empty(t, result.Agents[0].Codename, "should NOT see codename (requires secret)")
	require.Empty(t, result.Agents[0].Alias, "should NOT see alias (requires top_secret)")
	t.Log("Test passed: user with 'unclassified' level only sees unlabeled predicates")
}

// TestLabelAuthClassifiedLevel verifies that 'classified' level can access classified and below but not secret/top_secret.
func TestLabelAuthClassifiedLevel(t *testing.T) {
	t.Log("=== TestLabelAuthClassifiedLevel: User with 'classified' level - testing hierarchy ===")
	setupLabelAuthTest(t)
	dg := waitForCluster(t)

	// Create context with 'classified' security level via gRPC metadata
	ctx := auth.AttachToOutgoingContext(context.Background(), &auth.AuthContext{Level: "classified"})

	// Query for all predicates
	t.Log("Querying with 'classified' security level...")
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

	// 'classified' is below 'secret' in hierarchy, so should only see 'name' (unlabeled)
	// The schema uses @label(secret) and @label(top_secret), no @label(classified)
	var result struct {
		Agents []struct {
			Name     string `json:"name,omitempty"`
			Codename string `json:"codename,omitempty"`
			Alias    string `json:"alias,omitempty"`
		} `json:"agents"`
	}
	require.NoError(t, json.Unmarshal(resp.GetJson(), &result))

	require.Len(t, result.Agents, 1, "should have 1 agent")
	require.Equal(t, "James Bond", result.Agents[0].Name, "should see name (unlabeled)")
	require.Empty(t, result.Agents[0].Codename, "should NOT see codename (requires secret)")
	require.Empty(t, result.Agents[0].Alias, "should NOT see alias (requires top_secret)")
	t.Log("Test passed: user with 'classified' level only sees unlabeled predicates")
}

// TestLabelAuthWithoutACL verifies that label authorization works even when ACL is disabled.
func TestLabelAuthWithoutACL(t *testing.T) {
	t.Log("=== TestLabelAuthWithoutACL: Label auth should work independently of ACL ===")
	// This test runs in the label systest which does NOT have ACL enabled
	// It verifies that label-based authorization is independent of the ACL system
	setupLabelAuthTest(t)
	dg := waitForCluster(t)

	// Even without ACL, label authorization should filter predicates
	ctx := auth.AttachToOutgoingContext(context.Background(), &auth.AuthContext{Level: "secret"})

	t.Log("Querying with 'secret' level (ACL disabled)...")
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

	var result struct {
		Agents []struct {
			Name     string `json:"name,omitempty"`
			Codename string `json:"codename,omitempty"`
			Alias    string `json:"alias,omitempty"`
		} `json:"agents"`
	}
	require.NoError(t, json.Unmarshal(resp.GetJson(), &result))

	// Should see name and codename, not alias
	require.Len(t, result.Agents, 1)
	require.Equal(t, "James Bond", result.Agents[0].Name)
	require.Equal(t, "007", result.Agents[0].Codename)
	require.Empty(t, result.Agents[0].Alias)
	t.Log("Test passed: label auth works without ACL enabled")
}
