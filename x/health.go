/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package x

import (
	"sync/atomic"

	"github.com/pkg/errors"
)

var (
	// the drainingMode variable should be accessed through the atomic.Store and atomic.Load
	// functions. The value 0 means the draining-mode is disabled, and the value 1 means the
	// mode is enabled
	drainingMode uint32

	healthCheck     uint32
	errHealth       = errors.New("Please retry again, server is not ready to accept requests")
	errDrainingMode = errors.New("the server is in draining mode " +
		"and client requests will only be allowed after exiting the mode " +
		" by sending a GraphQL draining(enable: false) mutation to /admin")
)

// UpdateHealthStatus updates the server's health status so it can start accepting requests.
func UpdateHealthStatus(ok bool) {
	setStatus(&healthCheck, ok)
}

// UpdateDrainingMode updates the server's draining mode
func UpdateDrainingMode(enable bool) {
	setStatus(&drainingMode, enable)
}

// HealthCheck returns whether the server is ready to accept requests or not
// Load balancer would add the node to the endpoint once health check starts
// returning true
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
