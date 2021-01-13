package audit

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/golang/glog"

	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
)

type AuditEvent struct {
	User        string              `json:"user"`
	ServerHost  string              `json:"server_host"`
	ClientHost  string              `json:"client_host"`
	Timestamp   int64               `json:"timestamp"`
	Req         string              `json:"req"`
	Status      int                 `json:"status"`
	QueryParams map[string][]string `json:"query_params"`
	TimeTaken   int64               `json:"time_taken"`
}

const (
	UnauthorisedUser = "UnauthorisedUser"
	NoUser           = "NoUser"
	UnknownUser      = "UnknownUser"
)

var Auditor *AuditLogger

// todo add file rotation etc
type AuditLogger struct {
	log            *log.Logger
	AuditorEnabled bool
}

func InitAuditor() error {
	if !worker.EnterpriseEnabled() {
		return errors.New("enterprise features are disabled. You can enable them by " +
			"supplying the appropriate license file to Dgraph Zero using the HTTP endpoint")
	}

	e, err := os.OpenFile(worker.Config.AuditDir+"/dgraph_audit.json",
		os.O_WRONLY|os.O_CREATE|os.O_APPEND,
		0666)
	if err != nil {
		return err
	}

	Auditor = &AuditLogger{
		log:            log.New(e, "", 0),
		AuditorEnabled: true,
	}

	//Auditor.startListeningToLogAudit()
	return nil
}

func (a *AuditLogger) Close() {
	glog.Infof("Closing auditor")
}

func (a *AuditLogger) MaybeAuditFromCtx(w *ResponseWriter, r *http.Request, startTime int64) {
	var userId string
	var err error
	token := r.Header.Get("X-Dgraph-AccessToken")
	if token == "" {
		if x.WorkerConfig.AclEnabled {
			userId = UnauthorisedUser
		} else {
			userId = NoUser
		}
	} else {
		userId, err = x.ExtractUserName(token)
		if err != nil {
			userId = UnknownUser
		}
	}

	all, _ := ioutil.ReadAll(r.Body)
	event := &AuditEvent{
		User:        userId,
		ServerHost:  x.WorkerConfig.MyAddr,
		ClientHost:  r.RemoteAddr,
		Timestamp:   time.Now().Unix(),
		Req:         string(all),
		Status:      w.statusCode,
		QueryParams: r.URL.Query(),
		TimeTaken:   time.Now().UnixNano() - startTime,
	}
	a.MaybeAuditFromEvent(event)
}

func (a *AuditLogger) MaybeAuditFromEvent(event *AuditEvent) {
	if !a.AuditorEnabled {
		return
	}
	b, _ := json.Marshal(event)
	a.log.Print(string(b))
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
