package audit

import (
	"github.com/spf13/viper"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/golang/glog"

	"github.com/dgraph-io/dgraph/x"
)

var auditEnabled uint32

type AuditEvent struct {
	User        string              `json:"user"`
	ServerHost  string              `json:"server_host"`
	ClientHost  string              `json:"client_host"`
	Endpoint    string              `json:"endpoint"`
	Req         string              `json:"req"`
	Status      int                 `json:"status"`
	QueryParams map[string][]string `json:"query_params"`
	TimeTaken   int64               `json:"time_taken"`
}

const (
	UnauthorisedUser = "UnauthorisedUser"
	UnknownUser      = "UnknownUser"
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
		tick: time.NewTicker(time.Second * 5),
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

func trackIfEEValid(eeEnabledFunc func() bool, dir string) {
	for {
		select {
		case <-auditor.tick.C:
			if !eeEnabledFunc() {
				if atomic.LoadUint32(&auditEnabled) != 0 {
					atomic.StoreUint32(&auditEnabled, 0)
					glog.Infof("audit logs are disabled")
					auditor.log.Sync()
					auditor.log = nil
				}
				continue
			}

			if atomic.LoadUint32(&auditEnabled) != 1 {
				auditor.log = initlog(dir)
				atomic.StoreUint32(&auditEnabled, 1)
				glog.Info("audit logs are enabled")
			}
		}
	}
}

func Close() {
	auditor.tick.Stop()
	auditor.log.Sync()
}

func (a *auditLogger) AuditEvent(event *AuditEvent) {
	a.log.AuditI(event.Endpoint,
		"user", event.User,
		"server", event.ServerHost,
		"client", event.ClientHost,
		"req_body", event.Req,
		"query_param", event.QueryParams,
		"status", event.Status,
		"time", event.TimeTaken)
}

type ResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func NewResponseWriter(w http.ResponseWriter) *ResponseWriter {
	// WriteHeader(int) is not called if our response implicitly returns 200 OK, so
	// we default to that status code.
	return &ResponseWriter{w, http.StatusOK}
}

func (lrw *ResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}
