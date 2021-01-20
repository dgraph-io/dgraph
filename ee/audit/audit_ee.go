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
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
)

const (
	defaultAuditFilename = "dgraph_audit.log"
)

var auditEnabled uint32

type AuditEvent struct {
	User        string
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
	PoorManAuth      = "PoorManAuth"
	Grpc             = "Grpc"
	Http             = "Http"
)

var auditor *auditLogger = &auditLogger{}

type auditLogger struct {
	log  *x.Logger
	tick *time.Ticker
}

func ReadAuditEncKey(conf string) ([]byte, error) {
	encFile := x.GetFlagString(conf, "encrypt-file")
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
func InitAuditorIfNecessary(conf string, eeEnabled func() bool) error {
	if conf == "" {
		return nil
	}
	encKey, err := ReadAuditEncKey(conf)
	if err != nil {
		glog.Errorf("error while reading encryption file", err)
		return err
	}
	if eeEnabled() {
		if err := InitAuditor(x.GetFlagString(conf, "dir"), encKey); err != nil {
			return err
		}
	}
	auditor.tick = time.NewTicker(time.Minute * 5)
	go trackIfEEValid(x.GetFlagString(conf, "dir"), encKey, eeEnabled)
	return nil
}

// InitAuditor initializes the auditor.
// This method doesnt keep track of whether cluster is part of enterprise edition or not.
// Client has to keep track of that.
func InitAuditor(dir string, key []byte) error {
	var err error
	if auditor.log, err = x.InitLogger(dir, defaultAuditFilename, key); err != nil {
		return err
	}
	atomic.StoreUint32(&auditEnabled, 1)
	glog.Infoln("audit logs are enabled")
	return nil
}

// trackIfEEValid tracks enterprise license of the cluster.
// Right now alpha doesn't know about the enterprise/licence.
// That's why we needed to track if the current node is part of enterprise edition cluster
func trackIfEEValid(dir string, key []byte, eeEnabledFunc func() bool) {
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
				if auditor.log, err = x.InitLogger(dir, defaultAuditFilename, key); err != nil {
					continue
				}
				atomic.StoreUint32(&auditEnabled, 1)
				glog.Infof("audit logs are enabled")
			}
		}
	}
}

// Close stops the ticker and sync the pending logs in buffer.
// It also sets the log to nil, because its being called by zero when license expires.
// If license added, InitLogger will take care of the file.
func Close() {
	if auditor.tick != nil {
		auditor.tick.Stop()
	}
	auditor.log.Sync()
	auditor.log = nil
}

func (a *auditLogger) Audit(event *AuditEvent) {
	a.log.AuditI(event.Endpoint,
		"user", event.User,
		"server", event.ServerHost,
		"client", event.ClientHost,
		"req_type", event.ReqType,
		"req_body", event.Req,
		"query_param", event.QueryParams,
		"status", event.Status)
}
