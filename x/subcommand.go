/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package x

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// SubCommand a represents a sub-command in the command-line interface.
type SubCommand struct {
	Cmd  *cobra.Command
	Conf *viper.Viper

	EnvPrefix string
}
