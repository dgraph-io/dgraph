/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
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
	healthCheck uint32
	memoryCheck uint32
	memoryErr   = errors.New("Please retry again, server's memory is at capacity")
	healthErr   = errors.New("Please retry again, server is not ready to accept requests")
)

func UpdateMemoryStatus(ok bool) {
	setStatus(&memoryCheck, ok)
}

func UpdateHealthStatus(ok bool) {
	setStatus(&healthCheck, ok)
}

func setStatus(v *uint32, ok bool) {
	if ok {
		atomic.StoreUint32(v, 1)
	} else {
		atomic.StoreUint32(v, 0)
	}
}

// HealthCheck returns whether the server is ready to accept requests or not
// Load balancer would add the node to the endpoint once health check starts
// returning true
func HealthCheck() error {
	if atomic.LoadUint32(&memoryCheck) == 0 {
		return memoryErr
	}
	if atomic.LoadUint32(&healthCheck) == 0 {
		return healthErr
	}
	return nil
}

func init() {
	memoryCheck = 1
}
