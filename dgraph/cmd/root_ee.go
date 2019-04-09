// +build !oss

/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
 */

package cmd

import (
	acl "github.com/dgraph-io/dgraph/ee/acl"
	"github.com/dgraph-io/dgraph/ee/backup"
)

func init() {
	// subcommands already has the default subcommands, we append to EE ones to that.
	subcommands = append(subcommands,
		&backup.Restore,
		&backup.LsBackup,
		&acl.CmdAcl,
	)
}
