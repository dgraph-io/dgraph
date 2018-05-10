/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package x

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type SubCommand struct {
	Cmd  *cobra.Command
	Conf *viper.Viper

	EnvPrefix string
}
