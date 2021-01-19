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
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"sync/atomic"

	"google.golang.org/grpc"
)

func AuditRequestGRPC(ctx context.Context, req interface{},
	info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	skip := func(method string) bool {
		skipApis := map[string]bool{
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
			// health server
			"Check": true,
			"Watch": true,
		}
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
