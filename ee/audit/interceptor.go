//go:build oss
// +build oss

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 */

package audit

import (
	"context"
	"net/http"

	"github.com/hypermodeinc/dgraph/v24/graphql/schema"

	"google.golang.org/grpc"
)

func AuditRequestGRPC(ctx context.Context, req interface{},
	info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	return handler(ctx, req)
}

func AuditRequestHttp(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}

func AuditWebSockets(ctx context.Context, req *schema.Request) {
	return
}
