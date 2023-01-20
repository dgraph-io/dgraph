//go:build !oss
// +build !oss

/*
 * Copyright 2022 Dgraph Labs, Inc. All rights reserved.
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
 */

package alpha

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/golang/glog"

	"github.com/dgraph-io/dgo/v210/protos/api"
	"github.com/dgraph-io/dgraph/edgraph"
	"github.com/dgraph-io/dgraph/x"
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
	if err := jwt.Unmarshal(resp.Json); err != nil {
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
