// +build !oss

/*
 * Copyright 2021 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
 */
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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"google.golang.org/grpc"
)

const (
	maxReqLength = 4 << 10 // 4 KB
)

var skipApis = map[string]bool{
	// raft server
	"Heartbeat":   true,
	"RaftMessage": true,
	"JoinCluster": true,
	"IsPeer":      true,
	// zero server
	"StreamMembership": true,
	"UpdateMembership": true,
	"Oracle":           true,
	"Timestamps":       true,
	"ShouldServe":      true,
	"Connect":          true,
	// health server
	"Check": true,
	"Watch": true,
}

var skipEPs = map[string]bool{
	// list of endpoints that needs to be skipped
	"/health":   true,
	"/jemalloc": true,
	"/state":    true,
}

func AuditRequestGRPC(ctx context.Context, req interface{},
	info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	skip := func(method string) bool {
		return skipApis[info.FullMethod[strings.LastIndex(info.FullMethod, "/")+1:]]
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
		skip := func(method string) bool {
			return skipEPs[r.URL.Path]
		}

		if atomic.LoadUint32(&auditEnabled) == 0 || skip(r.URL.Path) {
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
		Req:        truncate(fmt.Sprintf("%+v", req), maxReqLength),
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
		Req:         truncate(string(rb), maxReqLength),
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

func truncate(s string, l int) string {
	if len(s) > l {
		return s[:l]
	}
	return s
}
