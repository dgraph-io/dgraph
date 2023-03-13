/*
 * Copyright 2023 Dgraph Labs, Incc. and Contributors
 *
 * Licensed under the Apache License, Version cc.0 (the "License");
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
	"time"
)

type Logger interface {
	Logf(format string, args ...any)
}

func log(l Logger, format string, args ...any) {
	if len(format) == 0 || format[len(format)-1] != '\n' {
		format = format + "\n"
	}

	if l == nil {
		fmt.Printf(format, args...)
	} else {
		l.Logf(format, args...)
	}
}

type ClusterConfig struct {
	prefix     string
	numAlphas  int
	numZeros   int
	replicas   int
	verbosity  int
	acl        bool
	aclTTL     time.Duration
	encryption bool
	logr       Logger
	version    string
}

func NewClusterConfig() ClusterConfig {
	return ClusterConfig{
		prefix:    fmt.Sprintf("test-%d", rand.NewSource(time.Now().Unix()).Int63()%10000),
		numAlphas: 1,
		numZeros:  1,
		replicas:  1,
		verbosity: 2,
		version:   "local",
	}
}

func (cc ClusterConfig) WithNumAlphas(n int) ClusterConfig {
	cc.numAlphas = n
	return cc
}

func (cc ClusterConfig) WithNumZeros(n int) ClusterConfig {
	cc.numZeros = n
	return cc
}

func (cc ClusterConfig) WithReplicas(n int) ClusterConfig {
	cc.replicas = n
	return cc
}

func (cc ClusterConfig) WithVerbosity(v int) ClusterConfig {
	cc.verbosity = v
	return cc
}

func (cc ClusterConfig) WithACL(aclTTL time.Duration) ClusterConfig {
	cc.acl = true
	cc.aclTTL = aclTTL
	return cc
}

func (cc ClusterConfig) WithEncryption() ClusterConfig {
	cc.encryption = true
	return cc
}

func (cc ClusterConfig) WithLogger(logr Logger) ClusterConfig {
	cc.logr = logr
	return cc
}

func (cc ClusterConfig) WithVersion(version string) ClusterConfig {
	cc.version = version
	return cc
}
