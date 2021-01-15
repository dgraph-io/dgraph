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
	Grpc             = "Grpc"
	Http             = "Http"
)

var auditor *auditLogger

type auditLogger struct {
	log  *x.Logger
	tick *time.Ticker
}

func InitAuditorIfNecessary(conf *viper.Viper, eeEnabled func() bool) {
	if !conf.GetBool("audit_enabled") {
		return
	}
	auditor = &auditLogger{
		tick: time.NewTicker(time.Minute * 5),
	}
	if eeEnabled() {
		auditor.log = initlog(conf.GetString("audit_dir"))
		atomic.StoreUint32(&auditEnabled, 1)
		glog.Infoln("audit logs are enabled")
	}

	go trackIfEEValid(eeEnabled, conf.GetString("audit_dir"))
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

func Close() {
	if auditor == nil {
		return
	}
	auditor.tick.Stop()
	auditor.log.Sync()
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
