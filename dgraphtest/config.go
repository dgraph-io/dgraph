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

var UpgradeCombos = [][]string{
	// {"v20.11.3", "v23.0.0-rc1"},
	// {"v21.03.0", "v23.0.0-rc1"},
	{"v21.03.0-92-g0c9f60156", "v23.0.0-rc1"},
	{"v21.03.0-98-g19f71a78a-slash", "v23.0.0-rc1"},
	{"v21.03.0-99-g4a03c144a-slash", "v23.0.0-rc1"},
	{"v21.03.1", "v23.0.0-rc1"},
	{"v21.03.2", "v23.0.0-rc1"},
	{"v21.12.0", "v23.0.0-rc1"},
	{"v22.0.0", "v23.0.0-rc1"},
	{"v22.0.1", "v23.0.0-rc1"},
	{"v22.0.2", "v23.0.0-rc1"},
}

type ClusterConfig struct {
	prefix         string
	numAlphas      int
	numZeros       int
	replicas       int
	verbosity      int
	acl            bool
	aclTTL         time.Duration
	encryption     bool
	version        string
	volumes        map[string]string
	refillInterval time.Duration
	uidLease       int
}

func NewClusterConfig() ClusterConfig {
	prefix := fmt.Sprintf("test-%d", rand.NewSource(time.Now().Unix()).Int63()%10000)
	defaultBackupVol := fmt.Sprintf("%v_backup", prefix)
	defaultExportVol := fmt.Sprintf("%v_export", prefix)
	return ClusterConfig{
		prefix:         prefix,
		numAlphas:      1,
		numZeros:       1,
		replicas:       1,
		verbosity:      2,
		version:        localVersion,
		volumes:        map[string]string{DefaultBackupDir: defaultBackupVol, DefaultExportDir: defaultExportVol},
		refillInterval: 20 * time.Second,
		uidLease:       50,
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

func (cc ClusterConfig) WithVersion(version string) ClusterConfig {
	cc.version = version
	return cc
}

// WithAlphaVolume allows creating a shared volumes across alphas with
// name volname and mount directory specified as dir inside the container
func (cc ClusterConfig) WithAlphaVolume(volname, dir string) ClusterConfig {
	cc.volumes[dir] = volname
	return cc
}

func (cc ClusterConfig) WithRefillInterval(interval time.Duration) ClusterConfig {
	cc.refillInterval = interval * time.Second
	return cc
}

func (cc ClusterConfig) WithUidLease(uidLease int) ClusterConfig {
	cc.uidLease = uidLease
	return cc
}
