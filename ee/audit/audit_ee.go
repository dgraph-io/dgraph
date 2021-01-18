// +build !oss

/*
 * Copyright 2017-2021 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package audit

import (
	"context"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"io/ioutil"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	"github.com/spf13/viper"
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

// InitAuditorIfNecessary accepts conf and enterprise edition check function.
// This method keep tracks whether cluster is part of enterprise edition or not.
// It pools eeEnabled function every five minutes to check if the license is still valid or not.
func InitAuditorIfNecessary(conf *viper.Viper, eeEnabled func() bool) {
	if !conf.GetBool("audit_enabled") {
		return
	}
	if eeEnabled() {
		InitAuditor(conf.GetString("audit_dir"))
	}
	auditor.tick = time.NewTicker(time.Minute * 5)
	go trackIfEEValid(eeEnabled, conf.GetString("audit_dir"))
}

// InitAuditor initializes the auditor.
// This method doesnt keep track of whether cluster is part of enterprise edition or not.
// Client has to keep track of that.
func InitAuditor(dir string) {
	auditor.log = initlog(dir)
	atomic.StoreUint32(&auditEnabled, 1)
	glog.Infoln("audit logs are enabled")
}

func initlog(dir string) *x.Logger {
	logger, err := x.InitLogger(dir, "dgraph_audit.log")
	if err != nil {
		glog.Errorf("error while initiating auditor %v", err)
		return nil
	}
	return logger
}

// trackIfEEValid tracks enterprise license of the cluster.
// Right now alpha doesn't know about the enterprise/licence.
// That's why we needed to track if the current node is part of enterprise edition cluster
func trackIfEEValid(eeEnabledFunc func() bool, dir string) {
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
				auditor.log = initlog(dir)
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

func auditGrpc(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, err error) {
	clientHost := ""
	if p, ok := peer.FromContext(ctx); ok {
		clientHost = p.Addr.String()
	}

	userId := ""
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if t := md.Get("accessJwt"); len(t) > 0 {
			userId = getUserId(t[0], false)
		} else if t := md.Get("auth-token"); len(t) > 0 {
			userId = getUserId(t[0], true)
		}
	}

	cd := codes.Unknown
	if serr, ok := status.FromError(err); ok {
		cd = serr.Code()
	}
	auditor.Audit(&AuditEvent{
		User:       userId,
		ServerHost: x.WorkerConfig.MyAddr,
		ClientHost: clientHost,
		Endpoint:   info.FullMethod,
		ReqType:    Grpc,
		Req:        fmt.Sprintf("%v", req),
		Status:     cd.String(),
	})
}

func auditHttp(w *ResponseWriter, r *http.Request) {
	rb, err := ioutil.ReadAll(r.Body)
	if err != nil {
		rb = []byte(err.Error())
	}

	userId := ""
	if token := r.Header.Get("X-Dgraph-AccessToken"); token != "" {
		userId = getUserId(token, false)
	} else if token := r.Header.Get("X-Dgraph-AuthToken"); token != "" {
		userId = getUserId(token, true)
	} else {
		userId = getUserId("", false)
	}
	auditor.Audit(&AuditEvent{
		User:        userId,
		ServerHost:  x.WorkerConfig.MyAddr,
		ClientHost:  r.RemoteAddr,
		Endpoint:    r.URL.Path,
		ReqType:     Http,
		Req:         string(rb),
		Status:      http.StatusText(w.statusCode),
		QueryParams: r.URL.Query(),
	})
}

func getUserId(token string, poorman bool) string {
	if poorman {
		return PoorManAuth
	}
	var userId string
	var err error
	if token == "" {
		if x.WorkerConfig.AclEnabled {
			userId = UnauthorisedUser
		}
	} else {
		if userId, err = x.ExtractUserName(token); err != nil {
			userId = UnknownUser
		}
	}
	return userId
}
