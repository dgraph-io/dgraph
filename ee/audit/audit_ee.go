// +build !oss

/*
 * Copyright 2021 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
 */

package audit

import (
	"io/ioutil"
	"math"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/ristretto/z"

	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
)

const (
	defaultAuditConf     = "dir=; compress=false; encrypt-file=; days=10; size=100"
	defaultAuditFilename = "dgraph_audit.log"
)

var auditEnabled uint32

type AuditEvent struct {
	User        string
	Namespace   uint64
	ServerHost  string
	ClientHost  string
	Endpoint    string
	ReqType     string
	Req         string
	Status      string
	QueryParams map[string][]string
}

const (
	UnauthorisedUser = "UnauthorisedUser"
	UnknownUser      = "UnknownUser"
	UnknownNamespace = math.MaxUint64
	PoorManAuth      = "PoorManAuth"
	Grpc             = "Grpc"
	Http             = "Http"
)

var auditor *auditLogger = &auditLogger{}

type auditLogger struct {
	log    *x.Logger
	tick   *time.Ticker
	closer *z.Closer
}

func GetAuditConf(conf string) *x.LoggerConf {
	if conf == "" {
		return nil
	}
	auditFlag := z.NewSuperFlag(conf).MergeAndCheckDefault(defaultAuditConf)
	dir := auditFlag.GetString("dir")
	x.AssertTruef(dir != "", "dir flag is not provided for the audit logs")
	encBytes, err := readAuditEncKey(auditFlag)
	x.Check(err)
	return &x.LoggerConf{
		Compress:      auditFlag.GetBool("compress"),
		Dir:           dir,
		EncryptionKey: encBytes,
		Days:          auditFlag.GetInt64("days"),
		Size:          auditFlag.GetInt64("size"),
	}
}

func readAuditEncKey(conf *z.SuperFlag) ([]byte, error) {
	encFile := conf.GetString("encrypt-file")
	if encFile == "" {
		return nil, nil
	}
	path, err := filepath.Abs(encFile)
	if err != nil {
		return nil, err
	}
	encKey, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return encKey, nil
}

// InitAuditorIfNecessary accepts conf and enterprise edition check function.
// This method keep tracks whether cluster is part of enterprise edition or not.
// It pools eeEnabled function every five minutes to check if the license is still valid or not.
func InitAuditorIfNecessary(conf *x.LoggerConf, eeEnabled func() bool) error {
	if conf == nil {
		return nil
	}
	if eeEnabled() {
		if err := InitAuditor(conf); err != nil {
			return err
		}
	}
	auditor.tick = time.NewTicker(time.Minute * 5)
	auditor.closer = z.NewCloser(1)
	go trackIfEEValid(conf, eeEnabled)
	return nil
}

// InitAuditor initializes the auditor.
// This method doesnt keep track of whether cluster is part of enterprise edition or not.
// Client has to keep track of that.
func InitAuditor(conf *x.LoggerConf) error {
	var err error
	if auditor.log, err = x.InitLogger(conf, defaultAuditFilename); err != nil {
		return err
	}
	atomic.StoreUint32(&auditEnabled, 1)
	glog.Infoln("audit logs are enabled")
	return nil
}

// trackIfEEValid tracks enterprise license of the cluster.
// Right now alpha doesn't know about the enterprise/licence.
// That's why we needed to track if the current node is part of enterprise edition cluster
func trackIfEEValid(conf *x.LoggerConf, eeEnabledFunc func() bool) {
	defer auditor.closer.Done()
	var err error
	for {
		select {
		case <-auditor.tick.C:
			if !eeEnabledFunc() && atomic.CompareAndSwapUint32(&auditEnabled, 1, 0) {
				glog.Infof("audit logs are disabled")
				auditor.log.Sync()
				auditor.log = nil
				continue
			}

			if atomic.LoadUint32(&auditEnabled) != 1 {
				if auditor.log, err = x.InitLogger(conf, defaultAuditFilename); err != nil {
					continue
				}
				atomic.StoreUint32(&auditEnabled, 1)
				glog.Infof("audit logs are enabled")
			}
		case <-auditor.closer.HasBeenClosed():
			return
		}
	}
}

// Close stops the ticker and sync the pending logs in buffer.
// It also sets the log to nil, because its being called by zero when license expires.
// If license added, InitLogger will take care of the file.
func Close() {
	if atomic.LoadUint32(&auditEnabled) == 0 {
		return
	}
	if auditor.tick != nil {
		auditor.tick.Stop()
	}
	if auditor.closer != nil {
		auditor.closer.SignalAndWait()
	}
	auditor.log.Sync()
	auditor.log = nil
	glog.Infoln("audit logs are closed.")
}

func (a *auditLogger) Audit(event *AuditEvent) {
	a.log.AuditI(event.Endpoint,
		"user", event.User,
		"namespace", event.Namespace,
		"server", event.ServerHost,
		"client", event.ClientHost,
		"req_type", event.ReqType,
		"req_body", event.Req,
		"query_param", event.QueryParams,
		"status", event.Status)
}
