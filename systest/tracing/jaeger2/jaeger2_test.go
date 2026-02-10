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
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/dgraph-io/dgraph/v25/testutil"
)

// JaegerServicesResponse represents the response from Jaeger's /api/services endpoint
type JaegerServicesResponse struct {
	Data   []string `json:"data"`
	Total  int      `json:"total"`
	Errors []any    `json:"errors"`
}

// JaegerTracesResponse represents the response from Jaeger's /api/traces endpoint
type JaegerTracesResponse struct {
	Data   []any `json:"data"`
	Errors []any `json:"errors"`
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

// TestJaeger2UIAccessible verifies that the Jaeger 2.x UI is accessible
func TestJaeger2UIAccessible(t *testing.T) {
	jaegerAddr := testutil.ContainerAddr("jaeger", 16686)
	url := fmt.Sprintf("http://%s/", jaegerAddr)

	resp, err := http.Get(url)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode, "Jaeger 2.x UI should be accessible")
}

// TestJaeger2ReceivesTraces verifies that Jaeger 2.x receives traces from Dgraph
func TestJaeger2ReceivesTraces(t *testing.T) {
	jaegerAddr := testutil.ContainerAddr("jaeger", 16686)
	alphaAddr := testutil.ContainerAddr("alpha1", 9080)

	// Make a Dgraph query to generate traces
	dg, err := testutil.DgraphClient(alphaAddr)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Run a simple query to generate a trace
	_, err = dg.NewTxn().Query(ctx, `{ q(func: has(name)) { uid } }`)
	require.NoError(t, err)

	// Give Jaeger time to process the trace
	time.Sleep(3 * time.Second)

	// Check that Jaeger has received services
	servicesURL := fmt.Sprintf("http://%s/api/services", jaegerAddr)
	resp, err := http.Get(servicesURL)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var services JaegerServicesResponse
	err = json.Unmarshal(body, &services)
	require.NoError(t, err)

	t.Logf("Jaeger 2.x services: %v", services.Data)
	require.NotEmpty(t, services.Data, "Jaeger 2.x should have registered services")

	// Verify our custom service name appears
	found := false
	for _, svc := range services.Data {
		if svc == "alpha1" {
			found = true
			break
		}
	}
	require.True(t, found, "Service 'alpha1' should be registered in Jaeger 2.x, got: %v", services.Data)
}

// TestJaeger2TracesHaveSpans verifies that traces contain actual spans in Jaeger 2.x
func TestJaeger2TracesHaveSpans(t *testing.T) {
	jaegerAddr := testutil.ContainerAddr("jaeger", 16686)
	alphaAddr := testutil.ContainerAddr("alpha1", 9080)

	// Make a Dgraph query to generate traces
	dg, err := testutil.DgraphClient(alphaAddr)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Run a mutation and query to generate traces
	_, err = dg.NewTxn().Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`_:test <name> "trace-test-v2" .`),
		CommitNow: true,
	})
	require.NoError(t, err)

	_, err = dg.NewTxn().Query(ctx, `{ q(func: has(name)) { uid name } }`)
	require.NoError(t, err)

	// Give Jaeger time to process traces
	time.Sleep(3 * time.Second)

	// Query for traces from our service
	tracesURL := fmt.Sprintf("http://%s/api/traces?service=alpha1&limit=10", jaegerAddr)
	resp, err := http.Get(tracesURL)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var traces JaegerTracesResponse
	err = json.Unmarshal(body, &traces)
	require.NoError(t, err)

	t.Logf("Found %d traces for service alpha1 in Jaeger 2.x", len(traces.Data))
	require.NotEmpty(t, traces.Data, "Should have traces for alpha1 service in Jaeger 2.x")
}
