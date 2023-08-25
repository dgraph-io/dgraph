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
	"os"
	"time"
)

type UpgradeCombo struct {
	Before   string
	After    string
	Strategy UpgradeStrategy
}

func AllUpgradeCombos(v20 bool) []UpgradeCombo {
	fixedVersionCombos := []UpgradeCombo{
		// OPEN SOURCE RELEASES, 4fc9cfd => v23.1.0
		// v23.1.0 has one error modified which was fixed in commit 4fc9cfd after v23.1.0
		{"v21.03.0-92-g0c9f60156", "4fc9cfd", BackupRestore},
		{"v21.03.0-98-g19f71a78a-slash", "4fc9cfd", BackupRestore},
		{"v21.03.0-99-g4a03c144a-slash", "4fc9cfd", BackupRestore},
		{"v21.03.1", "4fc9cfd", BackupRestore},
		{"v21.03.2", "4fc9cfd", BackupRestore},
		{"v22.0.0", "4fc9cfd", BackupRestore},
		{"v22.0.1", "4fc9cfd", BackupRestore},
		{"v22.0.2", "4fc9cfd", BackupRestore},

		//  CLOUD VERSIONS
		{"e3d3e6290", "4fc9cfd", BackupRestore}, // v21.03.0-48-ge3d3e6290
		{"8b9e92314", "4fc9cfd", BackupRestore}, // v21.03.0-63-g8b9e92314
		{"dfa5daec1", "4fc9cfd", BackupRestore}, // v21.03.0-66-gdfa5daec1
		{"88e4aa07c", "4fc9cfd", BackupRestore}, // v21.03.0-69-g88e4aa07c
		{"d9df244fb", "4fc9cfd", BackupRestore}, // v21.03.0-73-gd9df244fb
		{"ed09b8cc1", "4fc9cfd", BackupRestore}, // v21.03.0-76-ged09b8cc1
		{"e4ad0b113", "4fc9cfd", BackupRestore}, // v21.03.0-78-ge4ad0b113
		{"83c9cbedc", "4fc9cfd", BackupRestore}, // v21.03.0-82-g83c9cbedc
		{"c5862ae2a", "4fc9cfd", BackupRestore}, // v21.03.0-84-gc5862ae2a

		// In place upgrade for cloud versions
		{"e3d3e6290", "4fc9cfd", InPlace}, // v21.03.0-48-ge3d3e6290
		{"8b9e92314", "4fc9cfd", InPlace}, // v21.03.0-63-g8b9e92314
		{"dfa5daec1", "4fc9cfd", InPlace}, // v21.03.0-66-gdfa5daec1
		{"88e4aa07c", "4fc9cfd", InPlace}, // v21.03.0-69-g88e4aa07c
		{"d9df244fb", "4fc9cfd", InPlace}, // v21.03.0-73-gd9df244fb
		{"ed09b8cc1", "4fc9cfd", InPlace}, // v21.03.0-76-ged09b8cc1
		{"e4ad0b113", "4fc9cfd", InPlace}, // v21.03.0-78-ge4ad0b113
		{"83c9cbedc", "4fc9cfd", InPlace}, // v21.03.0-82-g83c9cbedc
		{"c5862ae2a", "4fc9cfd", InPlace}, // v21.03.0-84-gc5862ae2a
		{"0c9f60156", "4fc9cfd", InPlace}, // v21.03.0-92-g0c9f60156
	}

	// In mainCombos list, we keep latest version to current HEAD as well as
	// older versions of dgraph to ensure that a change does not cause failures.
	mainCombos := []UpgradeCombo{
		{"v23.0.1", localVersion, BackupRestore},
		{"v23.0.1", localVersion, InPlace},
		{"v21.03.0", "4fc9cfd", BackupRestore},
	}

	if v20 {
		fixedVersionCombos = append(fixedVersionCombos, []UpgradeCombo{
			{"v20.11.2-rc1-25-g4400610b2", "4fc9cfd", BackupRestore},
			{"v20.11.2-rc1-23-gaf5030a5", "4fc9cfd", BackupRestore},
			{"v20.11.0-11-gb36b4862", "4fc9cfd", BackupRestore},
		}...)

		mainCombos = append(mainCombos, []UpgradeCombo{
			{"v20.11.2-rc1-16-g4d041a3a", localVersion, BackupRestore},
		}...)
	}

	if os.Getenv("DGRAPH_UPGRADE_MAIN_ONLY") == "true" {
		return mainCombos
	} else {
		return fixedVersionCombos
	}
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
	portOffset     int // exposed port offset for grpc/http port for both alpha/zero
	bulkOutDir     string
	featureFlags   []string
}

func NewClusterConfig() ClusterConfig {
	prefix := fmt.Sprintf("dgraphtest-%d", rand.NewSource(time.Now().UnixNano()).Int63()%1000000)
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
		portOffset:     -1,
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

// WithExposedPortOffset allows exposing the alpha/zero ports (5080, 6080, 8080 and 9080)
// to fixed ports (port (5080, 6080, 8080 and 9080) + offset + id (0, 1, 2 ...)) on the host.
func (cc ClusterConfig) WithExposedPortOffset(offset uint64) ClusterConfig {
	cc.portOffset = int(offset)
	return cc
}

// WithBulkLoadOutDir sets the out dir for the bulk loader. This ensures
// that the same p directory is used while setting up alphas.
func (cc ClusterConfig) WithBulkLoadOutDir(dir string) ClusterConfig {
	cc.bulkOutDir = dir
	return cc
}

// WithNormalizeCompatibilityMode sets the normalize-compatibility-mode feature flag for alpha
func (cc ClusterConfig) WithNormalizeCompatibilityMode(mode string) ClusterConfig {
	cc.featureFlags = append(cc.featureFlags, fmt.Sprintf("normalize-compatibility-mode=%v", mode))
	return cc
}
