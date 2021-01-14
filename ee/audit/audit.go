package audit

import (
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/glog"

	"github.com/dgraph-io/dgraph/worker"
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
	mu   sync.RWMutex
	tick *time.Ticker
	log  x.ILogger
}

func InitAuditorIfNecessary(dir string) {
	auditor = &auditLogger{
		mu: sync.RWMutex{},
		tick: time.NewTicker(time.Minute * 5),
	}

	if !worker.EnterpriseEnabled() {
		return
	}

	initlog := func() x.ILogger{
		logger, err := x.InitLogger(dir, "dgraph_audit.log")
		if err != nil {
			glog.Errorf("error while initiating auditor %v", err)
			return nil
		}
		return logger
	}
	auditor.log = initlog()
	for {
		select {
		case <- auditor.tick.C:
			if !worker.EnterpriseEnabled() {
				if atomic.LoadUint32(&auditEnabled) != 0 {
					atomic.StoreUint32(&auditEnabled, 0)
					auditor.mu.Lock()
					auditor.log = nil
					auditor.mu.Unlock()
					continue
				}
			}

			if atomic.LoadUint32(&auditEnabled) != 1 {
				atomic.StoreUint32(&auditEnabled, 1)
				auditor.mu.Lock()
				auditor.log = initlog()
				auditor.mu.Unlock()
			}
		}
	}
}

func Close() {
	glog.Info("Closing auditor")
	auditor.mu.Lock()
	defer auditor.mu.Unlock()
	auditor.tick.Stop()
	auditor.log.Sync()
}

func (a *auditLogger) AuditFromEvent(event *AuditEvent) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.log == nil {
		return
	}
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
