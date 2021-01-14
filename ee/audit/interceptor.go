package audit

import (
	"bytes"
	"context"
	"fmt"
	"github.com/dgraph-io/dgraph/x"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"io"
	"io/ioutil"
	"net/http"
	"sync/atomic"
	"time"
)

func AuditRequestGRPC(ctx context.Context, req interface{},
	info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	if atomic.LoadUint32(&auditEnabled) == 0 {
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
