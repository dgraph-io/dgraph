//go:build integration

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"context"
	"encoding/json"
	"fmt"
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

// setupProgramAuthTest sets up test data with program-labeled edges for program auth tests.
func setupProgramAuthTest(t *testing.T) {
	t.Log("Setting up program auth test...")
	dg := waitForCluster(t)
	ctx := context.Background()

	// Drop all and set up schema
	t.Log("Dropping all data...")
	require.NoError(t, dg.Alter(ctx, &api.Operation{DropAll: true}))

	// Simple schema without labels - we'll use program facets on the data
	t.Log("Applying schema...")
	schema := `
		name: string @index(term) .
		project: string @index(term) .
		location: string @index(term) .
	`
	require.NoError(t, dg.Alter(ctx, &api.Operation{Schema: schema}))
	t.Log("Waiting 2s for schema to propagate...")
	time.Sleep(2 * time.Second)
}

// TestProgramAuthMutationInjectsPrograms verifies that mutations automatically get program facets.
func TestProgramAuthMutationInjectsPrograms(t *testing.T) {
	t.Log("=== TestProgramAuthMutationInjectsPrograms: Mutations should auto-inject program facets ===")
	setupProgramAuthTest(t)
	dg := waitForCluster(t)

	// Create context with programs - these should be auto-injected as facets
	ctx := auth.AttachToOutgoingContext(context.Background(), &auth.AuthContext{
		Programs: []string{"ALPHA", "BRAVO"},
	})

	// Insert data with program context
	t.Log("Inserting data with programs ALPHA,BRAVO...")
	_, err := dg.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
			_:p1 <name> "Project Alpha-Bravo" .
			_:p1 <project> "classified" .
		`),
	})
	require.NoError(t, err)

	// Query with same programs - should see the data (program filtering is server-side)
	t.Log("Querying with matching programs...")
	resp, err := dg.NewTxn().Query(ctx, `
		{
			projects(func: has(name)) {
				name
				project
			}
		}
	`)
	require.NoError(t, err)
	t.Logf("Query response: %s", string(resp.GetJson()))

	// Verify data is returned (program facets are enforced server-side, not via @facets directive)
	var result struct {
		Projects []struct {
			Name    string `json:"name"`
			Project string `json:"project"`
		} `json:"projects"`
	}
	require.NoError(t, json.Unmarshal(resp.GetJson(), &result))
	require.Len(t, result.Projects, 1, "should have 1 project")
	require.Equal(t, "Project Alpha-Bravo", result.Projects[0].Name)
	require.Equal(t, "classified", result.Projects[0].Project)
	t.Log("Test passed: mutation injected programs and query returned data")
}

// TestProgramAuthFiltersByProgram verifies that queries filter data based on user programs.
func TestProgramAuthFiltersByProgram(t *testing.T) {
	t.Log("=== TestProgramAuthFiltersByProgram: Queries should filter based on user programs ===")
	setupProgramAuthTest(t)
	dg := waitForCluster(t)

	// Insert data with ALPHA program
	ctxAlpha := auth.AttachToOutgoingContext(context.Background(), &auth.AuthContext{
		Programs: []string{"ALPHA"},
	})
	t.Log("Inserting ALPHA project...")
	_, err := dg.NewTxn().Mutate(ctxAlpha, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`_:p1 <name> "Alpha Project" .`),
	})
	require.NoError(t, err)

	// Insert data with BRAVO program
	ctxBravo := auth.AttachToOutgoingContext(context.Background(), &auth.AuthContext{
		Programs: []string{"BRAVO"},
	})
	t.Log("Inserting BRAVO project...")
	_, err = dg.NewTxn().Mutate(ctxBravo, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`_:p2 <name> "Bravo Project" .`),
	})
	require.NoError(t, err)

	// Query with ALPHA program - should only see ALPHA data
	t.Log("Querying with ALPHA program...")
	resp, err := dg.NewTxn().Query(ctxAlpha, `
		{
			projects(func: has(name)) {
				name
			}
		}
	`)
	require.NoError(t, err)
	t.Logf("ALPHA query response: %s", string(resp.GetJson()))

	var alphaResult struct {
		Projects []struct {
			Name string `json:"name"`
		} `json:"projects"`
	}
	require.NoError(t, json.Unmarshal(resp.GetJson(), &alphaResult))
	require.Len(t, alphaResult.Projects, 1, "ALPHA user should see 1 project")
	require.Equal(t, "Alpha Project", alphaResult.Projects[0].Name)

	// Query with BRAVO program - should only see BRAVO data
	t.Log("Querying with BRAVO program...")
	resp, err = dg.NewTxn().Query(ctxBravo, `
		{
			projects(func: has(name)) {
				name
			}
		}
	`)
	require.NoError(t, err)
	t.Logf("BRAVO query response: %s", string(resp.GetJson()))

	var bravoResult struct {
		Projects []struct {
			Name string `json:"name"`
		} `json:"projects"`
	}
	require.NoError(t, json.Unmarshal(resp.GetJson(), &bravoResult))
	require.Len(t, bravoResult.Projects, 1, "BRAVO user should see 1 project")
	require.Equal(t, "Bravo Project", bravoResult.Projects[0].Name)

	t.Log("Test passed: queries correctly filter by program")
}

// TestProgramAuthNoPrograms verifies that users without programs only see public data.
func TestProgramAuthNoPrograms(t *testing.T) {
	t.Log("=== TestProgramAuthNoPrograms: Users without programs should only see public data ===")
	setupProgramAuthTest(t)
	dg := waitForCluster(t)

	// Insert data with ALPHA program (protected)
	ctxAlpha := auth.AttachToOutgoingContext(context.Background(), &auth.AuthContext{
		Programs: []string{"ALPHA"},
	})
	t.Log("Inserting ALPHA project (protected)...")
	_, err := dg.NewTxn().Mutate(ctxAlpha, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`_:p1 <name> "Alpha Project" .`),
	})
	require.NoError(t, err)

	// Insert data without programs (public - accessible to everyone)
	t.Log("Inserting public project (no programs)...")
	_, err = dg.NewTxn().Mutate(context.Background(), &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`_:p2 <name> "Public Project" .`),
	})
	require.NoError(t, err)

	// Query with empty programs (auth active but no programs) - should only see public data
	ctxNoPrograms := auth.AttachToOutgoingContext(context.Background(), &auth.AuthContext{
		Programs: []string{},
	})
	t.Log("Querying with auth active but no programs...")
	resp, err := dg.NewTxn().Query(ctxNoPrograms, `
		{
			projects(func: has(name)) {
				name
			}
		}
	`)
	require.NoError(t, err)
	t.Logf("No-program query response: %s", string(resp.GetJson()))

	var result struct {
		Projects []struct {
			Name string `json:"name"`
		} `json:"projects"`
	}
	require.NoError(t, json.Unmarshal(resp.GetJson(), &result))
	// Should only see public project (user has no programs, can't access program-protected data)
	require.Len(t, result.Projects, 1, "user without programs should only see public data")
	require.Equal(t, "Public Project", result.Projects[0].Name)
	t.Log("Test passed: users without programs only see public data")
}

// TestProgramAuthMultiplePrograms verifies OR logic for multiple programs.
func TestProgramAuthMultiplePrograms(t *testing.T) {
	t.Log("=== TestProgramAuthMultiplePrograms: Multiple programs use OR logic ===")
	setupProgramAuthTest(t)
	dg := waitForCluster(t)

	// Insert ALPHA project
	ctxAlpha := auth.AttachToOutgoingContext(context.Background(), &auth.AuthContext{
		Programs: []string{"ALPHA"},
	})
	_, err := dg.NewTxn().Mutate(ctxAlpha, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`_:p1 <name> "Alpha Only" .`),
	})
	require.NoError(t, err)

	// Insert BRAVO project
	ctxBravo := auth.AttachToOutgoingContext(context.Background(), &auth.AuthContext{
		Programs: []string{"BRAVO"},
	})
	_, err = dg.NewTxn().Mutate(ctxBravo, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`_:p2 <name> "Bravo Only" .`),
	})
	require.NoError(t, err)

	// Query with both ALPHA and BRAVO programs - should see both
	ctxBoth := auth.AttachToOutgoingContext(context.Background(), &auth.AuthContext{
		Programs: []string{"ALPHA", "BRAVO"},
	})
	t.Log("Querying with ALPHA and BRAVO programs...")
	resp, err := dg.NewTxn().Query(ctxBoth, `
		{
			projects(func: has(name)) {
				name
			}
		}
	`)
	require.NoError(t, err)
	t.Logf("Multi-program query response: %s", string(resp.GetJson()))

	var result struct {
		Projects []struct {
			Name string `json:"name"`
		} `json:"projects"`
	}
	require.NoError(t, json.Unmarshal(resp.GetJson(), &result))
	require.Len(t, result.Projects, 2, "user with both programs should see both projects")
	t.Log("Test passed: multiple programs use OR logic")
}

// TestProgramAuthEqFunction tests program auth with eq() comparison function.
func TestProgramAuthEqFunction(t *testing.T) {
	t.Log("=== TestProgramAuthEqFunction: eq() should respect program auth ===")
	setupProgramAuthTest(t)
	dg := waitForCluster(t)

	// Insert ALPHA project
	ctxAlpha := auth.AttachToOutgoingContext(context.Background(), &auth.AuthContext{
		Programs: []string{"ALPHA"},
	})
	_, err := dg.NewTxn().Mutate(ctxAlpha, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`_:p1 <name> "Alpha Project" .`),
	})
	require.NoError(t, err)

	// Insert BRAVO project with same name pattern
	ctxBravo := auth.AttachToOutgoingContext(context.Background(), &auth.AuthContext{
		Programs: []string{"BRAVO"},
	})
	_, err = dg.NewTxn().Mutate(ctxBravo, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`_:p2 <name> "Alpha Project" .`),
	})
	require.NoError(t, err)

	// Query with eq() using ALPHA program - should only see ALPHA's data
	t.Log("Querying with eq() and ALPHA program...")
	resp, err := dg.NewTxn().Query(ctxAlpha, `
		{
			projects(func: eq(name, "Alpha Project")) {
				name
			}
		}
	`)
	require.NoError(t, err)
	t.Logf("eq() query response: %s", string(resp.GetJson()))

	var result struct {
		Projects []struct {
			Name string `json:"name"`
		} `json:"projects"`
	}
	require.NoError(t, json.Unmarshal(resp.GetJson(), &result))
	require.Len(t, result.Projects, 1, "ALPHA user should see only 1 matching project")
	t.Log("Test passed: eq() respects program auth")
}

// TestProgramAuthTermSearch tests program auth with term search functions.
func TestProgramAuthTermSearch(t *testing.T) {
	t.Log("=== TestProgramAuthTermSearch: allofterms/anyofterms should respect program auth ===")
	setupProgramAuthTest(t)
	dg := waitForCluster(t)

	// Insert ALPHA project
	ctxAlpha := auth.AttachToOutgoingContext(context.Background(), &auth.AuthContext{
		Programs: []string{"ALPHA"},
	})
	_, err := dg.NewTxn().Mutate(ctxAlpha, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`_:p1 <name> "secret mission alpha" .`),
	})
	require.NoError(t, err)

	// Insert BRAVO project with overlapping terms
	ctxBravo := auth.AttachToOutgoingContext(context.Background(), &auth.AuthContext{
		Programs: []string{"BRAVO"},
	})
	_, err = dg.NewTxn().Mutate(ctxBravo, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`_:p2 <name> "secret mission bravo" .`),
	})
	require.NoError(t, err)

	// Query with anyofterms using ALPHA program
	t.Log("Querying with anyofterms() and ALPHA program...")
	resp, err := dg.NewTxn().Query(ctxAlpha, `
		{
			projects(func: anyofterms(name, "secret mission")) {
				name
			}
		}
	`)
	require.NoError(t, err)
	t.Logf("anyofterms() query response: %s", string(resp.GetJson()))

	var result struct {
		Projects []struct {
			Name string `json:"name"`
		} `json:"projects"`
	}
	require.NoError(t, json.Unmarshal(resp.GetJson(), &result))
	require.Len(t, result.Projects, 1, "ALPHA user should see only their project")
	require.Contains(t, result.Projects[0].Name, "alpha")
	t.Log("Test passed: term search respects program auth")
}

// TestProgramAuthRegexp tests program auth with regexp() function.
func TestProgramAuthRegexp(t *testing.T) {
	t.Log("=== TestProgramAuthRegexp: regexp() should respect program auth ===")
	dg := waitForCluster(t)
	ctx := context.Background()

	// Setup schema with trigram index for regexp
	require.NoError(t, dg.Alter(ctx, &api.Operation{DropAll: true}))
	schema := `
		name: string @index(trigram) .
	`
	require.NoError(t, dg.Alter(ctx, &api.Operation{Schema: schema}))
	time.Sleep(2 * time.Second)

	// Insert ALPHA project
	ctxAlpha := auth.AttachToOutgoingContext(context.Background(), &auth.AuthContext{
		Programs: []string{"ALPHA"},
	})
	_, err := dg.NewTxn().Mutate(ctxAlpha, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`_:p1 <name> "Project-Alpha-001" .`),
	})
	require.NoError(t, err)

	// Insert BRAVO project
	ctxBravo := auth.AttachToOutgoingContext(context.Background(), &auth.AuthContext{
		Programs: []string{"BRAVO"},
	})
	_, err = dg.NewTxn().Mutate(ctxBravo, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`_:p2 <name> "Project-Bravo-002" .`),
	})
	require.NoError(t, err)

	// Query with regexp using ALPHA program
	t.Log("Querying with regexp() and ALPHA program...")
	resp, err := dg.NewTxn().Query(ctxAlpha, `
		{
			projects(func: regexp(name, /Project-.*/)) {
				name
			}
		}
	`)
	require.NoError(t, err)
	t.Logf("regexp() query response: %s", string(resp.GetJson()))

	var result struct {
		Projects []struct {
			Name string `json:"name"`
		} `json:"projects"`
	}
	require.NoError(t, json.Unmarshal(resp.GetJson(), &result))
	require.Len(t, result.Projects, 1, "ALPHA user should see only their project")
	require.Contains(t, result.Projects[0].Name, "Alpha")
	t.Log("Test passed: regexp() respects program auth")
}

// TestProgramAuthMatch tests program auth with match() fuzzy function.
func TestProgramAuthMatch(t *testing.T) {
	t.Log("=== TestProgramAuthMatch: match() should respect program auth ===")
	dg := waitForCluster(t)
	ctx := context.Background()

	// Setup schema with trigram index for match
	require.NoError(t, dg.Alter(ctx, &api.Operation{DropAll: true}))
	schema := `
		name: string @index(trigram) .
	`
	require.NoError(t, dg.Alter(ctx, &api.Operation{Schema: schema}))
	time.Sleep(2 * time.Second)

	// Insert ALPHA project
	ctxAlpha := auth.AttachToOutgoingContext(context.Background(), &auth.AuthContext{
		Programs: []string{"ALPHA"},
	})
	_, err := dg.NewTxn().Mutate(ctxAlpha, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`_:p1 <name> "AlphaProject" .`),
	})
	require.NoError(t, err)

	// Insert BRAVO project
	ctxBravo := auth.AttachToOutgoingContext(context.Background(), &auth.AuthContext{
		Programs: []string{"BRAVO"},
	})
	_, err = dg.NewTxn().Mutate(ctxBravo, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`_:p2 <name> "BravoProject" .`),
	})
	require.NoError(t, err)

	// Query with match using ALPHA program (fuzzy match with edit distance 3)
	t.Log("Querying with match() and ALPHA program...")
	resp, err := dg.NewTxn().Query(ctxAlpha, `
		{
			projects(func: match(name, "AlphaProjct", 3)) {
				name
			}
		}
	`)
	require.NoError(t, err)
	t.Logf("match() query response: %s", string(resp.GetJson()))

	var result struct {
		Projects []struct {
			Name string `json:"name"`
		} `json:"projects"`
	}
	require.NoError(t, json.Unmarshal(resp.GetJson(), &result))
	require.Len(t, result.Projects, 1, "ALPHA user should see only their project")
	require.Equal(t, "AlphaProject", result.Projects[0].Name)
	t.Log("Test passed: match() respects program auth")
}

// TestProgramAuthUidTraversal tests program auth with UID traversal.
func TestProgramAuthUidTraversal(t *testing.T) {
	t.Log("=== TestProgramAuthUidTraversal: UID traversal should respect program auth ===")
	dg := waitForCluster(t)
	ctx := context.Background()

	// Setup schema with edges
	require.NoError(t, dg.Alter(ctx, &api.Operation{DropAll: true}))
	schema := `
		name: string @index(term) .
		member: [uid] .
	`
	require.NoError(t, dg.Alter(ctx, &api.Operation{Schema: schema}))
	time.Sleep(2 * time.Second)

	// Insert team and members with ALPHA program
	ctxAlpha := auth.AttachToOutgoingContext(context.Background(), &auth.AuthContext{
		Programs: []string{"ALPHA"},
	})
	resp, err := dg.NewTxn().Mutate(ctxAlpha, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
			_:team <name> "Alpha Team" .
			_:m1 <name> "Alice" .
			_:m2 <name> "Bob" .
			_:team <member> _:m1 .
			_:team <member> _:m2 .
		`),
	})
	require.NoError(t, err)
	teamUID := resp.Uids["team"]

	// Insert team with BRAVO program
	ctxBravo := auth.AttachToOutgoingContext(context.Background(), &auth.AuthContext{
		Programs: []string{"BRAVO"},
	})
	_, err = dg.NewTxn().Mutate(ctxBravo, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
			_:team2 <name> "Bravo Team" .
			_:m3 <name> "Charlie" .
			_:team2 <member> _:m3 .
		`),
	})
	require.NoError(t, err)

	// Query team by UID with ALPHA program - should see team and members
	t.Log("Querying team by UID with ALPHA program...")
	queryResp, err := dg.NewTxn().Query(ctxAlpha, fmt.Sprintf(`
		{
			team(func: uid(%s)) {
				name
				member {
					name
				}
			}
		}
	`, teamUID))
	require.NoError(t, err)
	t.Logf("UID traversal response: %s", string(queryResp.GetJson()))

	var result struct {
		Team []struct {
			Name   string `json:"name"`
			Member []struct {
				Name string `json:"name"`
			} `json:"member"`
		} `json:"team"`
	}
	require.NoError(t, json.Unmarshal(queryResp.GetJson(), &result))
	require.Len(t, result.Team, 1)
	require.Equal(t, "Alpha Team", result.Team[0].Name)
	require.Len(t, result.Team[0].Member, 2, "should see both members")
	t.Log("Test passed: UID traversal respects program auth")
}

// TestProgramAuthUpdateBlocked tests that users cannot update program-protected data they don't have access to.
func TestProgramAuthUpdateBlocked(t *testing.T) {
	t.Log("=== TestProgramAuthUpdateBlocked: Users cannot update data they can't access ===")
	setupProgramAuthTest(t)
	dg := waitForCluster(t)

	// Insert data with ALPHA program
	ctxAlpha := auth.AttachToOutgoingContext(context.Background(), &auth.AuthContext{
		Programs: []string{"ALPHA"},
	})
	resp, err := dg.NewTxn().Mutate(ctxAlpha, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`_:p1 <name> "Alpha Secret" .`),
	})
	require.NoError(t, err)
	uid := resp.Uids["p1"]
	t.Logf("Created ALPHA-protected node with UID: %s", uid)

	// Try to update with BRAVO program - should fail
	ctxBravo := auth.AttachToOutgoingContext(context.Background(), &auth.AuthContext{
		Programs: []string{"BRAVO"},
	})
	t.Log("Attempting to update ALPHA data with BRAVO credentials...")
	_, err = dg.NewTxn().Mutate(ctxBravo, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(fmt.Sprintf(`<%s> <name> "Hacked by BRAVO" .`, uid)),
	})
	require.Error(t, err, "BRAVO user should not be able to update ALPHA data")
	require.Contains(t, err.Error(), "not authorized", "error should indicate authorization failure")
	t.Log("Test passed: update correctly blocked")
}

// TestProgramAuthDeleteBlocked tests that users cannot delete program-protected data they don't have access to.
func TestProgramAuthDeleteBlocked(t *testing.T) {
	t.Log("=== TestProgramAuthDeleteBlocked: Users cannot delete data they can't access ===")
	setupProgramAuthTest(t)
	dg := waitForCluster(t)

	// Insert data with ALPHA program
	ctxAlpha := auth.AttachToOutgoingContext(context.Background(), &auth.AuthContext{
		Programs: []string{"ALPHA"},
	})
	resp, err := dg.NewTxn().Mutate(ctxAlpha, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`_:p1 <name> "Alpha Secret" .`),
	})
	require.NoError(t, err)
	uid := resp.Uids["p1"]
	t.Logf("Created ALPHA-protected node with UID: %s", uid)

	// Try to delete with BRAVO program - should fail
	ctxBravo := auth.AttachToOutgoingContext(context.Background(), &auth.AuthContext{
		Programs: []string{"BRAVO"},
	})
	t.Log("Attempting to delete ALPHA data with BRAVO credentials...")
	_, err = dg.NewTxn().Mutate(ctxBravo, &api.Mutation{
		CommitNow: true,
		DelNquads: []byte(fmt.Sprintf(`<%s> <name> * .`, uid)),
	})
	require.Error(t, err, "BRAVO user should not be able to delete ALPHA data")
	require.Contains(t, err.Error(), "not authorized", "error should indicate authorization failure")

	// Verify data still exists with ALPHA credentials
	queryResp, err := dg.NewTxn().Query(ctxAlpha, fmt.Sprintf(`
		{
			node(func: uid(%s)) {
				name
			}
		}
	`, uid))
	require.NoError(t, err)
	require.Contains(t, string(queryResp.GetJson()), "Alpha Secret", "data should still exist")
	t.Log("Test passed: delete correctly blocked")
}

// TestProgramAuthUpdateAllowed tests that authorized users can update their own data.
func TestProgramAuthUpdateAllowed(t *testing.T) {
	t.Log("=== TestProgramAuthUpdateAllowed: Users can update data they have access to ===")
	setupProgramAuthTest(t)
	dg := waitForCluster(t)

	// Insert data with ALPHA program
	ctxAlpha := auth.AttachToOutgoingContext(context.Background(), &auth.AuthContext{
		Programs: []string{"ALPHA"},
	})
	resp, err := dg.NewTxn().Mutate(ctxAlpha, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`_:p1 <name> "Original Name" .`),
	})
	require.NoError(t, err)
	uid := resp.Uids["p1"]

	// Update with same ALPHA program - should succeed
	t.Log("Updating with ALPHA credentials...")
	_, err = dg.NewTxn().Mutate(ctxAlpha, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(fmt.Sprintf(`<%s> <name> "Updated Name" .`, uid)),
	})
	require.NoError(t, err, "ALPHA user should be able to update ALPHA data")

	// Verify update
	queryResp, err := dg.NewTxn().Query(ctxAlpha, fmt.Sprintf(`
		{
			node(func: uid(%s)) {
				name
			}
		}
	`, uid))
	require.NoError(t, err)
	require.Contains(t, string(queryResp.GetJson()), "Updated Name")
	t.Log("Test passed: authorized update succeeded")
}

// TestProgramAuthNoAuthCannotModifyProtected tests that users without auth cannot modify protected data.
func TestProgramAuthNoAuthCannotModifyProtected(t *testing.T) {
	t.Log("=== TestProgramAuthNoAuthCannotModifyProtected: No-auth users cannot modify protected data ===")
	setupProgramAuthTest(t)
	dg := waitForCluster(t)

	// Insert data with ALPHA program
	ctxAlpha := auth.AttachToOutgoingContext(context.Background(), &auth.AuthContext{
		Programs: []string{"ALPHA"},
	})
	resp, err := dg.NewTxn().Mutate(ctxAlpha, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`_:p1 <name> "Protected Data" .`),
	})
	require.NoError(t, err)
	uid := resp.Uids["p1"]

	// Try to update with empty auth context (auth active but no programs)
	ctxEmpty := auth.AttachToOutgoingContext(context.Background(), &auth.AuthContext{
		Programs: []string{},
	})
	t.Log("Attempting to update protected data with empty auth...")
	_, err = dg.NewTxn().Mutate(ctxEmpty, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(fmt.Sprintf(`<%s> <name> "Attempted Overwrite" .`, uid)),
	})
	require.Error(t, err, "user with no programs should not be able to modify protected data")
	t.Log("Test passed: empty auth cannot modify protected data")
}

// TestProgramAuthComparisonOps tests program auth with comparison operators (lt, gt, le, ge).
func TestProgramAuthComparisonOps(t *testing.T) {
	t.Log("=== TestProgramAuthComparisonOps: comparison operators should respect program auth ===")
	dg := waitForCluster(t)
	ctx := context.Background()

	// Setup schema with int index
	require.NoError(t, dg.Alter(ctx, &api.Operation{DropAll: true}))
	schema := `
		name: string @index(term) .
		priority: int @index(int) .
	`
	require.NoError(t, dg.Alter(ctx, &api.Operation{Schema: schema}))
	time.Sleep(2 * time.Second)

	// Insert ALPHA projects with various priorities
	ctxAlpha := auth.AttachToOutgoingContext(context.Background(), &auth.AuthContext{
		Programs: []string{"ALPHA"},
	})
	_, err := dg.NewTxn().Mutate(ctxAlpha, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
			_:p1 <name> "Alpha High" .
			_:p1 <priority> "10" .
			_:p2 <name> "Alpha Low" .
			_:p2 <priority> "2" .
		`),
	})
	require.NoError(t, err)

	// Insert BRAVO project with high priority
	ctxBravo := auth.AttachToOutgoingContext(context.Background(), &auth.AuthContext{
		Programs: []string{"BRAVO"},
	})
	_, err = dg.NewTxn().Mutate(ctxBravo, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
			_:p3 <name> "Bravo High" .
			_:p3 <priority> "10" .
		`),
	})
	require.NoError(t, err)

	// Query with gt() using ALPHA program
	t.Log("Querying with gt() and ALPHA program...")
	resp, err := dg.NewTxn().Query(ctxAlpha, `
		{
			projects(func: gt(priority, 5)) {
				name
				priority
			}
		}
	`)
	require.NoError(t, err)
	t.Logf("gt() query response: %s", string(resp.GetJson()))

	var result struct {
		Projects []struct {
			Name     string `json:"name"`
			Priority int    `json:"priority"`
		} `json:"projects"`
	}
	require.NoError(t, json.Unmarshal(resp.GetJson(), &result))
	require.Len(t, result.Projects, 1, "ALPHA user should see only their high priority project")
	require.Equal(t, "Alpha High", result.Projects[0].Name)
	t.Log("Test passed: comparison operators respect program auth")
}
