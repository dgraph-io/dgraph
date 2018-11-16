/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/golang/glog"

	"github.com/dgraph-io/dgraph/edgraph"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
)

// handlerInit does some standard checks. Returns false if something is wrong.
func handlerInit(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method != method {
		x.SetStatus(w, x.ErrorInvalidMethod, "Invalid method")
		return false
	}

	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || (!ipInIPWhitelistRanges(ip) && !net.ParseIP(ip).IsLoopback()) {
		x.SetStatus(w, x.ErrorUnauthorized, fmt.Sprintf("Request from IP: %v", ip))
		return false
	}
	return true
}

func shutDownHandler(w http.ResponseWriter, r *http.Request) {
	if !handlerInit(w, r, http.MethodGet) {
		return
	}

	close(shutdownCh)
	w.Header().Set("Content-Type", "application/json")
	x.Check2(w.Write([]byte(`{"code": "Success", "message": "Server is shutting down"}`)))
}

func exportHandler(w http.ResponseWriter, r *http.Request) {
	if !handlerInit(w, r, http.MethodGet) {
		return
	}
	// Export logic can be moved to dgraphzero.
	if err := worker.ExportOverNetwork(context.Background()); err != nil {
		x.SetStatus(w, err.Error(), "Export failed.")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	x.Check2(w.Write([]byte(`{"code": "Success", "message": "Export completed."}`)))
}

func backupHandler(w http.ResponseWriter, r *http.Request) {
	if !handlerInit(w, r, http.MethodPost) {
		return
	}
	target := r.FormValue("destination")
	if target == "" {
		err := x.Errorf("You must specify a 'destination' value")
		x.SetStatus(w, err.Error(), "Backup failed.")
		return
	}
	if err := worker.BackupOverNetwork(context.Background(), target); err != nil {
		x.SetStatus(w, err.Error(), "Backup failed.")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	x.Check2(w.Write([]byte(`{"code": "Success", "message": "Backup completed."}`)))

}

func memoryLimitHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		memoryLimitGetHandler(w, r)
	case http.MethodPut:
		memoryLimitPutHandler(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func memoryLimitPutHandler(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	memoryMB, err := strconv.ParseFloat(string(body), 64)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if memoryMB < edgraph.MinAllottedMemory {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "lru_mb must be at least %.0f\n", edgraph.MinAllottedMemory)
		return
	}

	posting.Config.Mu.Lock()
	posting.Config.AllottedMemory = memoryMB
	posting.Config.Mu.Unlock()
	w.WriteHeader(http.StatusOK)
}

func memoryLimitGetHandler(w http.ResponseWriter, r *http.Request) {
	posting.Config.Mu.Lock()
	memoryMB := posting.Config.AllottedMemory
	posting.Config.Mu.Unlock()

	if _, err := fmt.Fprintln(w, memoryMB); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func ipInIPWhitelistRanges(ipString string) bool {
	ip := net.ParseIP(ipString)

	if ip == nil {
		return false
	}

	for _, ipRange := range worker.Config.WhiteListedIPRanges {
		if bytes.Compare(ip, ipRange.Lower) >= 0 && bytes.Compare(ip, ipRange.Upper) <= 0 {
			return true
		}
	}
	return false
}

// Server implements protos.DgraphAdminServer
type AdminServer struct{}

func (adminServer *AdminServer) DeleteUser(ctx context.Context,
	request *api.DeleteUserRequest) (resp *api.DeleteUserResponse, err error) {
	panic("implement me")
}

type AdminOptions struct {
	hmacSecret []byte
}

var adminConfig AdminOptions

func SetAdminConfiguration(newConfig AdminOptions) {
	adminConfig = newConfig
}

func (adminServer *AdminServer) LogIn(ctx context.Context,
	request *api.LogInRequest) (*api.LogInResponse, error) {
	txnContext := &api.TxnContext{}
	resp := &api.LogInResponse{
		Context: &api.TxnContext{},
	}

	dbUser, err := queryDBUser(ctx, txnContext, request.Userid)
	if err != nil {
		glog.Infof("Unable to login user with user id: %v", request.Userid)
		return nil, err
	}

	if dbUser.Password != request.Password {
		glog.Infof("Password mismatch for user: %v", request.Userid)
		resp.Code = api.AclResponseCode_UNAUTHENTICATED
		return resp, nil
	}

	jwt := &Jwt{
		Header: StdJwtHeader,
		Payload:JwtPayload{
			Userid: request.Userid,
			Groups: toJwtGroups(dbUser.Groups),
			Exp: time.Now().AddDate(0, 0, 30).Unix(), // set the jwt valid for 30 days
		},
	}


	resp.Context.Jwt, err = jwt.EncodeToString(adminConfig.hmacSecret)
	if err != nil {
		glog.Errorf("Unable to encode jwt to string: %v", err)
		return nil, err
	}
	resp.Code = api.AclResponseCode_OK
	return resp, nil
}

func (adminServer *AdminServer) CreateUser(ctx context.Context,
	request *api.CreateUserRequest) (*api.CreateUserResponse, error) {
	err := validateCreateUserRequest(request)
	if err != nil {
		glog.Errorf("Error while validating create user request: %v", err)
		return nil, err
	}

	// initiating a transaction on the server side
	txnContext := &api.TxnContext{}

	dbUser, err := queryDBUser(ctx, txnContext, request.User.Userid)
	if err != nil {
		return nil, err
	}

	resp := &api.CreateUserResponse{}
	if dbUser != nil {
		resp.Uid = dbUser.Uid
		resp.Code = api.AclResponseCode_CONFLICT
		return resp, nil
	}

	createUserNQuads := getCreateUserNQuads(request)
	mu := &api.Mutation{
		StartTs:   txnContext.StartTs, // required so that the query and mutation is run as a single transaction
		CommitNow: true,
		Set:       createUserNQuads,
	}

	assignedIds, err := (&edgraph.Server{}).Mutate(ctx, mu)
	if err != nil {
		glog.Errorf("Unable to create user: %v", err)
		return nil, err
	}
	dgo.MergeContext(txnContext, assignedIds.Context)

	resp.Uid = assignedIds.Uids[x.NewUserLabel]
	glog.Infof("Created new user with id %v", request.User.Userid)
	return resp, nil
}

// parse the response and check existing of the uid
type DBGroup struct {
	Uid string `json:"uid"`
	GroupID string `json:"dgraph.xid"`
}

func toJwtGroups(groups []DBGroup) []JwtGroup {
	jwtGroups := make([]JwtGroup, len(groups))

	for _, g := range groups {
		jwtGroups = append(jwtGroups, JwtGroup{
			Group: g.GroupID,
			Wildcardacl: "", // TODO set it to the wild card acl returned from DB
		})
	}
	return jwtGroups
}

type DBUser struct {
	Uid string `json:"uid"`
	UserID string `json:"dgraph.xid"`
	Password string `json:"dgraph.password"`
	Groups []DBGroup `json:"dgraph.user.group"`
}


func queryDBUser(ctx context.Context, txnContext *api.TxnContext,
	userid string) (dbUser *DBUser, err error) {
	queryUid := `
    query search($userid: string){
      user(func: eq(` + x.Acl_XId + `, $userid)) {
	    uid,
        `+x.Acl_Password+`
        `+x.Acl_UserGroup+` {
          uid
          dgraph.xid
        }
      }
    }`

	queryVars := make(map[string]string)
	queryVars["$userid"] = userid
	queryRequest := api.Request{
		Query:    queryUid,
		Vars:     queryVars,
	}

	queryResp, err := (&edgraph.Server{}).Query(ctx, &queryRequest)
	if err != nil {
		glog.Errorf("Error while query user with id %s: %v", userid, err)
		return nil, err
	}
	// merge the response context so that the startTs and other metadata are populated to the txnContext
	dgo.MergeContext(txnContext, queryResp.GetTxn())

	m := make(map[string][]DBUser)

	err = json.Unmarshal(queryResp.GetJson(), &m)
	if err != nil {
		glog.Errorf("Unable to unmarshal the query user response for user", userid)
		return nil, err
	}
	users := m["user"]
	if len(users) == 0 {
		// the user does not exist
		return nil, nil
	}

	dbUser = &users[0]
	// populate the UserID field manually since it's not in the query response
	dbUser.UserID = userid

	return dbUser, nil
}

func getCreateUserNQuads(request *api.CreateUserRequest) []*api.NQuad {
	createUserNQuads := []*api.NQuad{
		{
			Subject:     "_:" + x.NewUserLabel,
			Predicate:   x.Acl_XId,
			ObjectValue: &api.Value{Val: &api.Value_StrVal{StrVal: request.User.Userid}},
		},
		{
			Subject:     "_:" + x.NewUserLabel,
			Predicate:   x.Acl_Password,
			ObjectValue: &api.Value{Val: &api.Value_StrVal{StrVal: request.User.Password}},
		}}

	// TODO: encode the user's attrs as a json blob and store under the x.Acl_UserBlob predicate
	return createUserNQuads
}

func validateCreateUserRequest(request *api.CreateUserRequest) error {
	if len(request.User.Userid) == 0 {
		return fmt.Errorf("The userid must not be empty.")
	}
	if len(request.User.Password) == 0 {
		return fmt.Errorf("The password must not be empty.")
	}
	return nil
}
