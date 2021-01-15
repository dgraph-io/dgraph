package audit

import (
	"sync/atomic"
	"time"

	"github.com/spf13/viper"

	"github.com/golang/glog"

	"github.com/dgraph-io/dgraph/x"
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
		InitAuditor(true, conf.GetString("audit_dir"))
	}
	auditor.tick = time.NewTicker(time.Minute * 5)
	go trackIfEEValid(eeEnabled, conf.GetString("audit_dir"))
}

// InitAuditor initializes the auditor.
// This method doesnt keep track of whether cluster is part of enterprise edition or not.
// Client has to keep track of that.
func InitAuditor(enabled bool, dir string) {
	if !enabled{
		return
	}
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

func (a *auditLogger) AuditEvent(event *AuditEvent) {
	a.log.AuditI(event.Endpoint,
		"user", event.User,
		"server", event.ServerHost,
		"client", event.ClientHost,
		"req_type", event.ReqType,
		"req_body", event.Req,
		"query_param", event.QueryParams,
		"status", event.Status)
}
