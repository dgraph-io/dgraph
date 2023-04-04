/*
 * Copyright 2017-2023 Dgraph Labs, Inc. and Contributors
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

package zero

import (
	"time"

	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/ristretto/z"
	"github.com/spf13/pflag"
)

func FillFlags(flag *pflag.FlagSet) {
	flag.IntP("port_offset", "o", 0,
		"Value added to all listening port numbers. [Grpc=5080, HTTP=6080]")
	flag.Int("replicas", 1, "How many Dgraph Alpha replicas to run per data shard group."+
		" The count includes the original shard.")
	flag.String("peer", "", "Address of another dgraphzero server.")
	flag.StringP("wal", "w", "zw", "Directory storing WAL.")
	flag.Duration("rebalance_interval", 8*time.Minute, "Interval for trying a predicate move.")
	flag.String("enterprise_license", "", "Path to the enterprise license file.")
	flag.String("cid", "", "Cluster ID")

	flag.String("limit", worker.ZeroLimitsDefaults, z.NewSuperFlagHelp(worker.ZeroLimitsDefaults).
		Head("Limit options").
		Flag("uid-lease",
			`The maximum number of UIDs that can be leased by namespace (except default namespace)
			in an interval specified by refill-interval. Set it to 0 to remove limiting.`).
		Flag("refill-interval",
			"The interval after which the tokens for UID lease are replenished.").
		Flag("disable-admin-http",
			"Turn on/off the administrative endpoints exposed over Zero's HTTP port.").
		String())

	flag.String("raft", raftDefaults, z.NewSuperFlagHelp(raftDefaults).
		Head("Raft options").
		Flag("idx",
			"Provides an optional Raft ID that this Alpha would use to join Raft groups.").
		Flag("learner",
			`Make this Zero a "learner" node. In learner mode, this Zero will not participate `+
				"in Raft elections. This can be used to achieve a read-only replica.").
		String())

	flag.String("audit", worker.AuditDefaults, z.NewSuperFlagHelp(worker.AuditDefaults).
		Head("Audit options").
		Flag("output",
			`[stdout, /path/to/dir] This specifies where audit logs should be output to.
			"stdout" is for standard output. You can also specify the directory where audit logs
			will be saved. When stdout is specified as output other fields will be ignored.`).
		Flag("compress",
			"Enables the compression of old audit logs.").
		Flag("encrypt-file",
			"The path to the key file to be used for audit log encryption.").
		Flag("days",
			"The number of days audit logs will be preserved.").
		Flag("size",
			"The audit log max size in MB after which it will be rolled over.").
		String())
}