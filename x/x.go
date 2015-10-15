package x

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Sirupsen/logrus"
)

const (
	E_OK               = "E_OK"
	E_UNAUTHORIZED     = "E_UNAUTHORIZED"
	E_INVALID_METHOD   = "E_INVALID_METHOD"
	E_INVALID_REQUEST  = "E_INVALID_REQUEST"
	E_MISSING_REQUIRED = "E_MISSING_REQUIRED"
	E_ERROR            = "E_ERROR"
	E_NODATA           = "E_NODATA"
	E_UPTODATE         = "E_UPTODATE"
	E_NOPERMISSION     = "E_NOPERMISSION"

	DUMMY_UUID = "00000000-0000-0000-0000-000000000000"
)

type Status struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Triple struct {
	Entity    uint64
	EntityEid string
	Attribute string
	Value     interface{}
	ValueId   uint64
	Source    string
	Timestamp time.Time
}

func Log(p string) *logrus.Entry {
	l := logrus.WithFields(logrus.Fields{
		"package": p,
	})
	return l
}

func Err(entry *logrus.Entry, err error) *logrus.Entry {
	return entry.WithField("error", err)
}

func SetStatus(w http.ResponseWriter, code, msg string) {
	r := &Status{Code: code, Message: msg}
	if js, err := json.Marshal(r); err == nil {
		fmt.Fprint(w, string(js))
	} else {
		panic(fmt.Sprintf("Unable to marshal: %+v", r))
	}
}

func Reply(w http.ResponseWriter, rep interface{}) {
	if js, err := json.Marshal(rep); err == nil {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, string(js))
	} else {
		SetStatus(w, E_ERROR, "Internal server error")
	}
}

func ParseRequest(w http.ResponseWriter, r *http.Request, data interface{}) bool {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&data); err != nil {
		SetStatus(w, E_ERROR, fmt.Sprintf("While parsing request: %v", err))
		return false
	}
	return true
}
