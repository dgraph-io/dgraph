package audit

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/dgraph/x"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

func AuditRequestGRPC(ctx context.Context, req interface{},
	info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	skip := func(method string) bool {
		skipApis := []string{"Heartbeat", "RaftMessage", "JoinCluster", "IsPeer", // raft server
			"StreamMembership", "UpdateMembership", "Oracle", // zero server
			"Check", "Watch", // health server
		}

		for _, api := range skipApis {
			if strings.HasSuffix(method, api) {
				return true
			}
		}
		return false
	}

	if atomic.LoadUint32(&auditEnabled) == 0 || skip(info.FullMethod) {
		return handler(ctx, req)
	}

	startTime := time.Now().UnixNano()
	response, err := handler(ctx, req)
	maybeAuditGRPC(ctx, req, info, err, startTime)
	return response, err
}

func AuditRequestHttp(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.LoadUint32(&auditEnabled) == 0 {
			next.ServeHTTP(w, r)
			return
		}
		startTime := time.Now().UnixNano()
		lrw := NewResponseWriter(w)
		var buf bytes.Buffer
		tee := io.TeeReader(r.Body, &buf)
		r.Body = ioutil.NopCloser(tee)
		next.ServeHTTP(lrw, r)
		r.Body = ioutil.NopCloser(bytes.NewReader(buf.Bytes()))
		maybeAuditHttp(lrw, r, startTime)
	})
}

func maybeAuditGRPC(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, err error,
	startTime int64) {
	var code codes.Code
	if serr, ok := status.FromError(err); !ok {
		code = codes.Unknown
	} else {
		code = serr.Code()
	}
	clientHost := ""
	p, ok := peer.FromContext(ctx)
	if ok {
		clientHost = p.Addr.String()
	}

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		md = metadata.New(nil)
	}

	token := ""
	t := md.Get("accessJwt")
	if len(t) > 0 {
		token = t[0]
	}
	event := &AuditEvent{
		User:       getUserId(token),
		ServerHost: x.WorkerConfig.MyAddr,
		ClientHost: clientHost,
		Endpoint:   info.FullMethod,
		ReqType:    Grpc,
		Req:        fmt.Sprintf("%v", req),
		Status:     int(code),
		TimeTaken:  time.Now().UnixNano() - startTime,
	}
	auditor.AuditEvent(event)
}

func maybeAuditHttp(w *ResponseWriter, r *http.Request, startTime int64) {
	all, _ := ioutil.ReadAll(r.Body)
	event := &AuditEvent{
		User:        getUserId(r.Header.Get("X-Dgraph-AccessToken")),
		ServerHost:  x.WorkerConfig.MyAddr,
		ClientHost:  r.RemoteAddr,
		Endpoint:    r.URL.Path,
		ReqType:     Http,
		Req:         string(all),
		Status:      w.statusCode,
		QueryParams: r.URL.Query(),
		TimeTaken:   time.Now().UnixNano() - startTime,
	}
	auditor.AuditEvent(event)
}

func getUserId(token string) string {
	var userId string
	var err error
	if token == "" {
		if x.WorkerConfig.AclEnabled {
			userId = UnauthorisedUser
		}
	} else {
		userId, err = x.ExtractUserName(token)
		if err != nil {
			userId = UnknownUser
		}
	}
	return userId
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

func (rw *ResponseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
