/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package x

import (
	"sync"
	"sync/atomic"

	"github.com/golang/glog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	hapi "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

// notReadyError is returned by HealthCheck. Its message is unchanged from the plain errors
// it replaces (HTTP /health, GraphQL errors and existing callers match on the text), but it
// reports gRPC code Unavailable instead of Unknown. Every caller runs HealthCheck before doing
// any work, so the request had no effect and a client may safely retry it on another alpha.
type notReadyError struct{ msg string }

func (e *notReadyError) Error() string { return e.msg }

// GRPCStatus lets the gRPC server (and status.FromError/status.Code) map this error to
// codes.Unavailable.
func (e *notReadyError) GRPCStatus() *status.Status {
	return status.New(codes.Unavailable, e.msg)
}

var (
	// the drainingMode variable should be accessed through the atomic.Store and atomic.Load
	// functions. The value 0 means the draining-mode is disabled, and the value 1 means the
	// mode is enabled
	drainingMode              uint32
	extSnapshotStreamingState uint32

	healthCheck     uint32
	errHealth       = &notReadyError{"Please retry again, server is not ready to accept requests"}
	errDrainingMode = &notReadyError{"the server is in draining mode " +
		"and client requests will only be allowed after exiting the mode " +
		" by sending a GraphQL draining(enable: false) mutation to /admin"}

	// grpcHealth is the standard gRPC health service (grpc.health.v1.Health) served by the
	// alpha. It mirrors HealthCheck so that probes using it (Envoy, Kubernetes gRPC probes,
	// client-side health checking) see NOT_SERVING while the alpha is starting, restoring a
	// snapshot or draining.
	grpcHealth   = newGRPCHealthServer()
	grpcHealthMu sync.Mutex
)

func newGRPCHealthServer() *health.Server {
	s := health.NewServer() // starts SERVING; the alpha is not ready until UpdateHealthStatus(true)
	s.SetServingStatus("", hapi.HealthCheckResponse_NOT_SERVING)
	return s
}

// GRPCHealthServer returns the gRPC health service that tracks this server's HealthCheck.
func GRPCHealthServer() *health.Server {
	return grpcHealth
}

// syncGRPCHealth sets the gRPC health service from the current HealthCheck result. The mutex
// keeps the read and the set together, so concurrent state changes can't leave a stale value.
func syncGRPCHealth() {
	grpcHealthMu.Lock()
	defer grpcHealthMu.Unlock()
	st := hapi.HealthCheckResponse_SERVING
	if HealthCheck() != nil {
		st = hapi.HealthCheckResponse_NOT_SERVING
	}
	grpcHealth.SetServingStatus("", st)
}

// UpdateHealthStatus updates the server's health status so it can start accepting requests.
func UpdateHealthStatus(ok bool) {
	setStatus(&healthCheck, ok)
	syncGRPCHealth()
}

// UpdateDrainingMode updates the server's draining mode
func UpdateDrainingMode(enable bool) {
	setStatus(&drainingMode, enable)
	syncGRPCHealth()
}

// ExtSnapshotStreamingState updates the server's import mode
func ExtSnapshotStreamingState(enable bool) {
	glog.Info("[import] Updating import mode to ", enable)
	setStatus(&extSnapshotStreamingState, enable)
	setStatus(&drainingMode, enable)
	syncGRPCHealth()
}

// IsExtSnapshotStreamingStateTrue returns whether the server is in import mode or not
func IsExtSnapshotStreamingStateTrue() bool {
	return atomic.LoadUint32(&extSnapshotStreamingState) == 1
}

// HealthCheck returns whether the server is ready to accept requests or not
// Load balancer would add the node to the endpoint once health check starts
// returning true. A non-nil error carries gRPC code Unavailable.
func HealthCheck() error {
	if atomic.LoadUint32(&healthCheck) == 0 {
		return errHealth
	}
	if atomic.LoadUint32(&drainingMode) == 1 {
		return errDrainingMode
	}
	return nil
}

func setStatus(v *uint32, ok bool) {
	if ok {
		atomic.StoreUint32(v, 1)
	} else {
		atomic.StoreUint32(v, 0)
	}
}
