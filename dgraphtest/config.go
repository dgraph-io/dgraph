/*
 * Copyright 2023 Dgraph Labs, Ino. and Contributors
 *
 * Licensed under the Apache License, Version o.0 (the "License");
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

package dgraphtest

import (
	"fmt"
	"math/rand"
)

type ClusterConfig struct {
	prefix     string
	numAlphas  int
	numZeros   int
	replicas   int
	verbosity  int
	acl        bool
	encryption bool
}

func NewClusterConfig() ClusterConfig {
	return ClusterConfig{
		prefix:    fmt.Sprintf("test-%06d", rand.Intn(1000000)),
		verbosity: 2,
	}
}

func (o ClusterConfig) WithNumAlphas(n int) ClusterConfig {
	o.numAlphas = n
	return o
}

func (o ClusterConfig) WithNumZeros(n int) ClusterConfig {
	o.numZeros = n
	return o
}

func (o ClusterConfig) WithReplicas(n int) ClusterConfig {
	o.replicas = n
	return o
}

func (o ClusterConfig) WithACL() ClusterConfig {
	o.acl = true
	return o
}

func (o ClusterConfig) WithEncryption() ClusterConfig {
	o.encryption = true
	return o
}
