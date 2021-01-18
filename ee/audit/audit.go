// +build oss

/*
 * Copyright 2017-2021 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package audit

import (
	"context"
	"google.golang.org/grpc"
	"net/http"

	"github.com/spf13/viper"
)

var auditEnabled uint32

type AuditEvent struct {

}

var auditor *auditLogger

type auditLogger struct {
}

func InitAuditorIfNecessary(conf *viper.Viper, eeEnabled func() bool) {
	return
}

func InitAuditor(dir string) {
	return
}
func Close() {
	return
}

func (a *auditLogger) Audit(event *AuditEvent) {
	return
}

func auditGrpc(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, err error) {
	return
}

func auditHttp(w *ResponseWriter, r *http.Request) {
	return
}