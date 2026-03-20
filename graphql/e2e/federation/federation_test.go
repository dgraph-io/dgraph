//go:build integration

/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package federation

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/v25/graphql/e2e/common"
	"github.com/dgraph-io/dgraph/v25/testutil"
	"github.com/dgraph-io/dgraph/v25/x"
)

var (
	// Alpha1 serves the user-service subgraph
	Alpha1HTTP            = testutil.ContainerAddr("alpha1", 8080)
	Alpha1GraphqlURL      = "http://" + Alpha1HTTP + "/graphql"
	Alpha1GraphqlAdminURL = "http://" + Alpha1HTTP + "/admin"

	// Alpha2 serves the reviews-service subgraph
	Alpha2HTTP            = testutil.ContainerAddr("alpha2", 8080)
	Alpha2GraphqlURL      = "http://" + Alpha2HTTP + "/graphql"
	Alpha2GraphqlAdminURL = "http://" + Alpha2HTTP + "/admin"
)

// TestMain sets up the test environment with two Dgraph alpha instances
func TestMain(m *testing.M) {
	userSchema, err := os.ReadFile("testdata/user-service.graphql")
	x.Panic(err)

	reviewsSchema, err := os.ReadFile("testdata/reviews-service.graphql")
	x.Panic(err)

	// Wait for both alphas to be ready
	err = common.CheckGraphQLStarted(Alpha1GraphqlAdminURL)
	x.Panic(err)
	err = common.CheckGraphQLStarted(Alpha2GraphqlAdminURL)
	x.Panic(err)

	// Bootstrap alpha1 with user-service schema
	common.BootstrapServer(userSchema, nil)

	// Deploy reviews-service schema to alpha2
	// Retry until schema is successfully deployed
	maxRetries := 30
	for i := 0; i < maxRetries; i++ {
		resp, updateErr := http.Post("http://"+Alpha2HTTP+"/admin/schema",
			"application/graphql", strings.NewReader(string(reviewsSchema)))
		if updateErr == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				// Wait a bit for schema to be loaded
				time.Sleep(500 * time.Millisecond)
				break
			}
		}
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		if i == maxRetries-1 {
			x.Panic(fmt.Errorf("failed to deploy schema to alpha2 after %d retries", maxRetries))
		}
		time.Sleep(200 * time.Millisecond)
	}

	os.Exit(m.Run())
}

// TestRoverFederationComposition tests Apollo Federation supergraph composition
// with two Dgraph instances serving different federated subgraphs.
//
// This test:
// 1. Fetches SDL via _service query from alpha1 (user-service) and alpha2 (reviews-service)
// 2. Runs Apollo Rover CLI to compose a supergraph
// 3. Verifies composition succeeds (auxiliary types should be excluded for @extends types)
func TestRoverFederationComposition(t *testing.T) {

	// Test that we can run docker
	testCmd := exec.Command("docker", "version")
	if err := testCmd.Run(); err != nil {
		t.Skip("Skipping test: docker is not running or accessible")
	}

	testdataDir := "./testdata"
	absTestdataDir, err := filepath.Abs(testdataDir)
	require.NoError(t, err)

	// Fetch SDL from alpha1 (user-service)
	serviceQuery := &common.GraphQLParams{
		Query: `query { _service { sdl } }`,
	}

	userServiceResp := serviceQuery.ExecuteAsPost(t, Alpha1GraphqlURL)
	common.RequireNoGQLErrors(t, userServiceResp)

	var userServiceResult struct {
		Service struct {
			SDL string `json:"sdl"`
		} `json:"_service"`
	}
	require.NoError(t, json.Unmarshal(userServiceResp.Data, &userServiceResult))

	// Fetch SDL from alpha2 (reviews-service)
	reviewsServiceResp := serviceQuery.ExecuteAsPost(t, Alpha2GraphqlURL)
	common.RequireNoGQLErrors(t, reviewsServiceResp)

	var reviewsServiceResult struct {
		Service struct {
			SDL string `json:"sdl"`
		} `json:"_service"`
	}
	require.NoError(t, json.Unmarshal(reviewsServiceResp.Data, &reviewsServiceResult))

	userSDL := userServiceResult.Service.SDL
	reviewsSDL := reviewsServiceResult.Service.SDL

	// Write the SDL files that Rover will use
	userSDLFile := filepath.Join(testdataDir, "user-service-generated.graphql")
	require.NoError(t, os.WriteFile(userSDLFile, []byte(userSDL), 0644))
	defer os.Remove(userSDLFile)

	reviewsSDLFile := filepath.Join(testdataDir, "reviews-service-generated.graphql")
	require.NoError(t, os.WriteFile(reviewsSDLFile, []byte(reviewsSDL), 0644))
	defer os.Remove(reviewsSDLFile)

	// Create supergraph config that points to the generated SDL files
	supergraphConfig := fmt.Sprintf(`federation_version: =2.7.1
subgraphs:
  users:
    routing_url: %s
    schema:
      file: ./user-service-generated.graphql
  reviews:
    routing_url: %s
    schema:
      file: ./reviews-service-generated.graphql
`, Alpha1GraphqlURL, Alpha2GraphqlURL)
	supergraphFile := filepath.Join(testdataDir, "supergraph-generated.yml")
	require.NoError(t, os.WriteFile(supergraphFile, []byte(supergraphConfig), 0644))
	defer os.Remove(supergraphFile)

	// Run rover supergraph compose using Docker
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "run", "--rm",
		"-e", "APOLLO_ELV2_LICENSE=accept",
		"-v", absTestdataDir+":/workspace",
		"-w", "/workspace",
		"node:24-slim",
		"sh", "-c",
		`apt-get update -qq && apt-get install -y -qq curl > /dev/null 2>&1 && \
		 curl -sSL https://rover.apollo.dev/nix/latest | sh > /dev/null 2>&1 && \
		 $HOME/.rover/bin/rover supergraph compose --config supergraph-generated.yml`)

	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	// The auxiliary types for extended User should NOT be generated from alpha2 (reviews-service)
	if err != nil {
		if strings.Contains(outputStr, "UserPatch") ||
			strings.Contains(outputStr, "UserOrderable") ||
			strings.Contains(outputStr, "UserHasFilter") {
			t.Errorf("Rover composition failed with auxiliary type conflicts. "+
				"The fix should exclude these types for @extends. Error: %v", err)
		} else {
			t.Errorf("Rover composition failed with unexpected error: %v", err)
		}
	}
}
