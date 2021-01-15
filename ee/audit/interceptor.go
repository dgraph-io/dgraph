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
	response, err := handler(ctx, req)
	auditGrpc(ctx, req, info, err)
	return response, err
}

func AuditRequestHttp(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.LoadUint32(&auditEnabled) == 0 {
			next.ServeHTTP(w, r)
			return
		}

		rw := NewResponseWriter(w)
		var buf bytes.Buffer
		tee := io.TeeReader(r.Body, &buf)
		r.Body = ioutil.NopCloser(tee)
		next.ServeHTTP(rw, r)
		r.Body = ioutil.NopCloser(bytes.NewReader(buf.Bytes()))
		auditHttp(rw, r)
	})
}

func auditGrpc(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, err error) {
	var code codes.Code
	if serr, ok := status.FromError(err); !ok {
		code = codes.Unknown
	} else {
		code = serr.Code()
	}
	clientHost := ""
	if p, ok := peer.FromContext(ctx); ok {
		clientHost = p.Addr.String()
	}

	token := ""
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if t := md.Get("accessJwt"); len(t) > 0 {
			token = t[0]
		}
	}

	auditor.AuditEvent(&AuditEvent{
		User:       getUserId(token),
		ServerHost: x.WorkerConfig.MyAddr,
		ClientHost: clientHost,
		Endpoint:   info.FullMethod,
		ReqType:    Grpc,
		Req:        fmt.Sprintf("%v", req),
		Status:     code.String(),
	})
}

func auditHttp(w *ResponseWriter, r *http.Request) {
	rb, err := ioutil.ReadAll(r.Body)
	if err != nil {
		rb = []byte(err.Error())
	}
	auditor.AuditEvent(&AuditEvent{
		User:        getUserId(r.Header.Get("X-Dgraph-AccessToken")),
		ServerHost:  x.WorkerConfig.MyAddr,
		ClientHost:  r.RemoteAddr,
		Endpoint:    r.URL.Path,
		ReqType:     Http,
		Req:         string(rb),
		Status:      http.StatusText(w.statusCode),
		QueryParams: r.URL.Query(),
	})
}

func getUserId(token string) string {
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
