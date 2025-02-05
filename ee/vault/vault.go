//go:build oss
// +build oss

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 */

package vault

import (
	"github.com/golang/glog"
	"github.com/spf13/viper"

	"github.com/hypermodeinc/dgraph/v24/ee"
)

func GetKeys(config *viper.Viper) (*ee.Keys, error) {
	glog.Exit("flags: vault is an enterprise-only feature")
	return nil, nil
}
