//go:build oss
// +build oss

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 */

package ee

import (
	"fmt"

	"github.com/spf13/viper"
)

// GetKeys returns the ACL and encryption keys as configured by the user
// through the --acl, --encryption, and --vault flags. On OSS builds,
// this function always returns an error.
func GetKeys(config *viper.Viper) (*Keys, error) {
	return nil, fmt.Errorf(
		"flags: acl / encryption is an enterprise-only feature")
}
