//go:build !oss
// +build !oss

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 */

package cmd

import (
	acl "github.com/hypermodeinc/dgraph/v24/ee/acl"
	"github.com/hypermodeinc/dgraph/v24/ee/audit"
	"github.com/hypermodeinc/dgraph/v24/ee/backup"
)

func init() {
	// subcommands already has the default subcommands, we append to EE ones to that.
	subcommands = append(subcommands,
		&backup.Restore,
		&backup.LsBackup,
		&backup.ExportBackup,
		&acl.CmdAcl,
		&audit.CmdAudit,
	)
}
