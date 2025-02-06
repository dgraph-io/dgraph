//go:build !oss
// +build !oss

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 */

package alpha

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/golang/glog"
	"google.golang.org/protobuf/proto"

	"github.com/dgraph-io/dgo/v240/protos/api"
	"github.com/hypermodeinc/dgraph/v24/edgraph"
	"github.com/hypermodeinc/dgraph/v24/x"
)

func loginHandler(w http.ResponseWriter, r *http.Request) {
	if commonHandler(w, r) {
		return
	}

	// Pass in PoorMan's auth, IP information if present.
	ctx := x.AttachRemoteIP(context.Background(), r)
	ctx = x.AttachAuthToken(ctx, r)

	body := readRequest(w, r)
	loginReq := api.LoginRequest{}
	if err := json.Unmarshal(body, &loginReq); err != nil {
		x.SetStatusWithData(w, x.Error, err.Error())
		return
	}

	resp, err := (&edgraph.Server{}).Login(ctx, &loginReq)
	if err != nil {
		x.SetStatusWithData(w, x.ErrorInvalidRequest, err.Error())
		return
	}

	jwt := &api.Jwt{}
	if err := proto.Unmarshal(resp.Json, jwt); err != nil {
		x.SetStatusWithData(w, x.Error, err.Error())
	}

	response := map[string]interface{}{}
	mp := make(map[string]string)
	mp["accessJWT"] = jwt.AccessJwt
	mp["refreshJWT"] = jwt.RefreshJwt
	response["data"] = mp

	js, err := json.Marshal(response)
	if err != nil {
		x.SetStatusWithData(w, x.Error, err.Error())
		return
	}

	if _, err := x.WriteResponse(w, r, js); err != nil {
		glog.Errorf("Error while writing response: %v", err)
	}
}

func init() {
	http.HandleFunc("/login", loginHandler)
}
