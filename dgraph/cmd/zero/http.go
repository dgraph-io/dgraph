/*
 * Copyright 2023 Dgraph Labs, Inc. and Contributors
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

package zero

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/dgraph-io/dgo/v240/protos/api"
	"github.com/dgraph-io/dgraph/v24/protos/pb"
	"github.com/dgraph-io/dgraph/v24/x"
)

// intFromQueryParam checks for name as a query param, converts it to uint64 and returns it.
// It also writes any errors to w. A bool is returned to indicate if the param was parsed
// successfully.
func intFromQueryParam(w http.ResponseWriter, r *http.Request, name string) (uint64, bool) {
	str := r.URL.Query().Get(name)
	if len(str) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		x.SetStatus(w, x.ErrorInvalidRequest, fmt.Sprintf("%s not passed", name))
		return 0, false
	}
	val, err := strconv.ParseUint(str, 0, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		x.SetStatus(w, x.ErrorInvalidRequest, fmt.Sprintf("Error while parsing %s", name))
		return 0, false
	}
	return val, true
}

func (st *state) assign(w http.ResponseWriter, r *http.Request) {
	x.AddCorsHeaders(w)
	w.Header().Set("Content-Type", "application/json")
	if r.Method == "OPTIONS" {
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusBadRequest)
		x.SetStatus(w, x.ErrorInvalidMethod, "Invalid method")
		return
	}
	val, ok := intFromQueryParam(w, r, "num")
	if !ok {
		return
	}

	num := &pb.Num{Val: val}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var ids *pb.AssignedIds
	var err error
	what := r.URL.Query().Get("what")
	switch what {
	case "uids":
		num.Type = pb.Num_UID
		ids, err = st.zero.AssignIds(ctx, num)
	case "timestamps":
		num.Type = pb.Num_TXN_TS
		if num.Val == 0 {
			num.ReadOnly = true
		}
		ids, err = st.zero.Timestamps(ctx, num)
	case "nsids":
		num.Type = pb.Num_NS_ID
		ids, err = st.zero.AssignIds(ctx, num)
	default:
		x.SetStatus(w, x.Error,
			fmt.Sprintf("Invalid what: [%s]. Must be one of: [uids, timestamps, nsids]", what))
		return
	}
	if err != nil {
		x.SetStatus(w, x.Error, err.Error())
		return
	}

	m := protojson.MarshalOptions{EmitUnpopulated: true}
	buf, err := m.Marshal(ids)
	if err != nil {
		x.SetStatus(w, x.ErrorNoData, err.Error())
		return
	}

	if _, err := w.Write(buf); err != nil {
		x.SetStatus(w, x.Error, err.Error())
		return
	}
}

// removeNode can be used to remove a node from the cluster. It takes in the RAFT id of the node
// and the group it belongs to. It can be used to remove Dgraph alpha and Zero nodes(group=0).
func (st *state) removeNode(w http.ResponseWriter, r *http.Request) {
	x.AddCorsHeaders(w)
	if r.Method == "OPTIONS" {
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusBadRequest)
		x.SetStatus(w, x.ErrorInvalidMethod, "Invalid method")
		return
	}

	nodeId, ok := intFromQueryParam(w, r, "id")
	if !ok {
		return
	}
	groupId, ok := intFromQueryParam(w, r, "group")
	if !ok {
		return
	}

	if _, err := st.zero.RemoveNode(
		context.Background(),
		&pb.RemoveNodeRequest{NodeId: nodeId, GroupId: uint32(groupId)},
	); err != nil {
		x.SetStatus(w, x.Error, err.Error())
		return
	}
	_, err := fmt.Fprintf(w, "Removed node with group: %v, idx: %v", groupId, nodeId)
	if err != nil {
		glog.Warningf("Error while writing response: %+v", err)
	}
}

// moveTablet can be used to move a tablet to a specific group. It takes in tablet and group as
// argument.
func (st *state) moveTablet(w http.ResponseWriter, r *http.Request) {
	x.AddCorsHeaders(w)
	if r.Method == "OPTIONS" {
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusBadRequest)
		x.SetStatus(w, x.ErrorInvalidMethod, "Invalid method")
		return
	}

	if !st.node.AmLeader() {
		w.WriteHeader(http.StatusBadRequest)
		x.SetStatus(w, x.ErrorInvalidRequest,
			"This Zero server is not the leader. Re-run command on leader.")
		return
	}

	namespace := r.URL.Query().Get("namespace")
	namespace = strings.TrimSpace(namespace)
	ns := x.GalaxyNamespace
	if namespace != "" {
		var err error
		if ns, err = strconv.ParseUint(namespace, 0, 64); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			x.SetStatus(w, x.ErrorInvalidRequest, "Invalid namespace in query parameter.")
			return
		}
	}

	tablet := r.URL.Query().Get("tablet")
	if len(tablet) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		x.SetStatus(w, x.ErrorInvalidRequest, "tablet is a mandatory query parameter")
		return
	}

	groupId, ok := intFromQueryParam(w, r, "group")
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		x.SetStatus(w, x.ErrorInvalidRequest,
			"Query parameter 'group' should contain a valid integer.")
		return
	}
	dstGroup := uint32(groupId)

	var resp *pb.Status
	var err error
	if resp, err = st.zero.MoveTablet(
		context.Background(),
		&pb.MoveTabletRequest{Namespace: ns, Tablet: tablet, DstGroup: dstGroup},
	); err != nil {
		if resp.GetMsg() == x.ErrorInvalidRequest {
			w.WriteHeader(http.StatusBadRequest)
			x.SetStatus(w, x.ErrorInvalidRequest, err.Error())
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			x.SetStatus(w, x.Error, err.Error())
		}
		return
	}
	_, err = fmt.Fprint(w, resp.GetMsg())
	if err != nil {
		glog.Warningf("Error while writing response: %+v", err)
	}
}

func (st *state) getState(w http.ResponseWriter, r *http.Request) {
	x.AddCorsHeaders(w)
	w.Header().Set("Content-Type", "application/json")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := st.node.WaitLinearizableRead(ctx); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		x.SetStatus(w, x.Error, err.Error())
		return
	}
	mstate := st.zero.membershipState()
	if mstate == nil {
		x.SetStatus(w, x.ErrorNoData, "No membership state found.")
		return
	}

	m := protojson.MarshalOptions{EmitUnpopulated: true}
	buf, err := m.Marshal(mstate)
	if err != nil {
		x.SetStatus(w, x.ErrorNoData, err.Error())
		return
	}
	if _, err := w.Write(buf); err != nil {
		x.SetStatus(w, x.Error, err.Error())
		return
	}
}

func (s *Server) zeroHealth(ctx context.Context) (*api.Response, error) {
	if ctx.Err() != nil {
		return nil, errors.Wrap(ctx.Err(), "http request context error")
	}
	health := pb.HealthInfo{
		Instance: "zero",
		Address:  x.WorkerConfig.MyAddr,
		Status:   "healthy",
		Version:  x.Version(),
		Uptime:   int64(time.Since(x.WorkerConfig.StartTime) / time.Second),
		LastEcho: time.Now().Unix(),
	}
	jsonOut, err := json.Marshal(health)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to marshal zero health, error")
	}
	return &api.Response{Json: jsonOut}, nil
}

func (st *state) pingResponse(w http.ResponseWriter, r *http.Request) {
	x.AddCorsHeaders(w)

	/*
	 * zero is changed to also output the health in JSON format for client
	 * request header "Accept: application/json".
	 *
	 * Backward compatibility- Before this change the '/health' endpoint
	 * used to output the string OK. After the fix it returns OK when the
	 * client sends the request without "Accept: application/json" in its
	 * http header.
	 */
	switch r.Header.Get("Accept") {
	case "application/json":
		resp, err := (st.zero).zeroHealth(r.Context())
		if err != nil {
			x.SetStatus(w, x.Error, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(resp.Json); err != nil {
			glog.Warningf("http error send failed, error msg=[%v]", err)
			return
		}
	default:
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("OK")); err != nil {
			glog.Warningf("http error send failed, error msg=[%v]", err)
			return
		}
	}
}
