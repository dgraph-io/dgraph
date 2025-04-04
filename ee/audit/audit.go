//go:build oss
// +build oss

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 */

package audit

import "github.com/hypermodeinc/dgraph/v24/x"

type AuditConf struct {
	Dir string
}

func GetAuditConf(conf string) *x.LoggerConf {
	return nil
}

func InitAuditorIfNecessary(conf *x.LoggerConf) error {
	return nil
}

func InitAuditor(conf *x.LoggerConf, gId, nId uint64) error {
	return nil
}

func Close() {
	return
}
