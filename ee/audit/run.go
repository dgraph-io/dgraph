//go:build oss
// +build oss

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 */

package audit

import (
	"github.com/spf13/cobra"

	"github.com/hypermodeinc/dgraph/v24/x"
)

var CmdAudit x.SubCommand

func init() {
	CmdAudit.Cmd = &cobra.Command{
		Use:   "audit",
		Short: "Enterprise feature. Not supported in oss version",
	}
}
