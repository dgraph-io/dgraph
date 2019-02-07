// +build !oss

/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
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

package alpha

import (
	"bytes"
	"context"
	"fmt"
	"net/http"

	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/edgraph"
	"github.com/dgraph-io/dgraph/x"
)

func loginHandler(w http.ResponseWriter, r *http.Request) {
	if commonHandler(w, r) {
		return
	}

	user := r.Header.Get("X-Dgraph-User")
	password := r.Header.Get("X-Dgraph-Password")
	refreshJwt := r.Header.Get("X-Dgraph-RefreshJWT")
	ctx := context.Background()
	resp, err := (&edgraph.Server{}).Login(ctx, &api.LoginRequest{
		Userid:       user,
		Password:     password,
		RefreshToken: refreshJwt,
	})

	if err != nil {
		x.SetStatusWithData(w, x.ErrorInvalidRequest, err.Error())
		return
	}

	jwt := &api.Jwt{}
	if err := jwt.Unmarshal(resp.Json); err != nil {
		x.SetStatusWithData(w, x.Error, err.Error())
	}

	var out bytes.Buffer
	out.WriteString(fmt.Sprintf("ACCESS JWT:\n%s\n", jwt.AccessJwt))
	out.WriteString(fmt.Sprintf("REFRESH JWT:\n%s\n", jwt.RefreshJwt))
	writeResponse(w, r, out.Bytes())
}

func init() {
	http.HandleFunc("/login", loginHandler)
}
