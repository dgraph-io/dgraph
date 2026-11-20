/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package x

import (
	"context"
	"fmt"
	"net"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	hapi "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func grpcHealthStatus(t *testing.T) hapi.HealthCheckResponse_ServingStatus {
	t.Helper()
	resp, err := GRPCHealthServer().Check(context.Background(), &hapi.HealthCheckRequest{})
	require.NoError(t, err)
	return resp.Status
}

func resetHealthState(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		ExtSnapshotStreamingState(false)
		UpdateDrainingMode(false)
		UpdateHealthStatus(false)
	})
}

func TestHealthCheckNotReadyIsUnavailable(t *testing.T) {
	resetHealthState(t)
	UpdateHealthStatus(false)
	UpdateDrainingMode(false)

	err := HealthCheck()
	require.Error(t, err)
	// The message is unchanged: HTTP /health, GraphQL and existing callers match on it.
	require.Equal(t, "Please retry again, server is not ready to accept requests", err.Error())
	require.Equal(t, codes.Unavailable, status.Code(err))
	require.Equal(t, hapi.HealthCheckResponse_NOT_SERVING, grpcHealthStatus(t))
}

func TestHealthCheckDrainingIsUnavailable(t *testing.T) {
	resetHealthState(t)
	UpdateHealthStatus(true)
	UpdateDrainingMode(true)

	err := HealthCheck()
	require.Error(t, err)
	require.Contains(t, err.Error(), "the server is in draining mode")
	require.Equal(t, codes.Unavailable, status.Code(err))
	require.Equal(t, hapi.HealthCheckResponse_NOT_SERVING, grpcHealthStatus(t))
}

func TestHealthCheckReady(t *testing.T) {
	resetHealthState(t)
	UpdateHealthStatus(true)
	UpdateDrainingMode(false)

	require.NoError(t, HealthCheck())
	require.Equal(t, hapi.HealthCheckResponse_SERVING, grpcHealthStatus(t))
}

func TestGRPCHealthFollowsTransitions(t *testing.T) {
	resetHealthState(t)
	steps := []struct {
		name string
		do   func()
		want hapi.HealthCheckResponse_ServingStatus
	}{
		{"ready", func() { UpdateHealthStatus(true) }, hapi.HealthCheckResponse_SERVING},
		{"snapshot restore", func() { UpdateHealthStatus(false) }, hapi.HealthCheckResponse_NOT_SERVING},
		{"snapshot done", func() { UpdateHealthStatus(true) }, hapi.HealthCheckResponse_SERVING},
		{"drain on", func() { UpdateDrainingMode(true) }, hapi.HealthCheckResponse_NOT_SERVING},
		{"drain off", func() { UpdateDrainingMode(false) }, hapi.HealthCheckResponse_SERVING},
		{"import on", func() { ExtSnapshotStreamingState(true) }, hapi.HealthCheckResponse_NOT_SERVING},
		{"import off", func() { ExtSnapshotStreamingState(false) }, hapi.HealthCheckResponse_SERVING},
		// Draining while not ready: leaving draining must not report SERVING.
		{"not ready", func() { UpdateHealthStatus(false) }, hapi.HealthCheckResponse_NOT_SERVING},
		{"drain on, not ready", func() { UpdateDrainingMode(true) }, hapi.HealthCheckResponse_NOT_SERVING},
		{"drain off, not ready", func() { UpdateDrainingMode(false) }, hapi.HealthCheckResponse_NOT_SERVING},
	}
	for _, s := range steps {
		s.do()
		require.Equal(t, s.want, grpcHealthStatus(t), s.name)
	}
}

// Callers that wrap the error keep code Unavailable (grpc-go unwraps to find GRPCStatus).
func TestHealthCheckWrappedKeepsCode(t *testing.T) {
	resetHealthState(t)
	UpdateHealthStatus(false)

	err := HealthCheck()
	require.Equal(t, codes.Unavailable, status.Code(errors.Wrap(err, "while doing query")))
	require.Equal(t, codes.Unavailable, status.Code(fmt.Errorf("while doing query: %w", err)))
}

// Over the wire: a handler that returns HealthCheck's error reaches the client as
// Unavailable with the original message, and the health service reports NOT_SERVING.
func TestHealthCheckOverGRPC(t *testing.T) {
	resetHealthState(t)
	UpdateHealthStatus(true)
	UpdateDrainingMode(true)

	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer(grpc.UnknownServiceHandler(
		func(any, grpc.ServerStream) error { return HealthCheck() }))
	hapi.RegisterHealthServer(srv, GRPCHealthServer())
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()

	err = conn.Invoke(context.Background(), "/api.Dgraph/CheckVersion",
		&hapi.HealthCheckRequest{}, &hapi.HealthCheckResponse{})
	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.Unavailable, st.Code())
	require.Contains(t, st.Message(), "the server is in draining mode")

	resp, err := hapi.NewHealthClient(conn).Check(context.Background(), &hapi.HealthCheckRequest{})
	require.NoError(t, err)
	require.Equal(t, hapi.HealthCheckResponse_NOT_SERVING, resp.Status)
}
