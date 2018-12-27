// +build !oss

/*
 * Copyright 2018 Dgraph Labs, Inc. All rights reserved.
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
 */

package edgraph

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"google.golang.org/grpc/metadata"

	"github.com/dgraph-io/dgraph/gql"

	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/ee/acl"
	"github.com/dgrijalva/jwt-go"
	"github.com/golang/glog"
	"google.golang.org/grpc/peer"

	otrace "go.opencensus.io/trace"
)

func (s *Server) Login(ctx context.Context,
	request *api.LoginRequest) (*api.Response, error) {
	ctx, span := otrace.StartSpan(ctx, "server.Login")
	defer span.End()

	// record the client ip for this login request
	var addr string
	if ip, ok := peer.FromContext(ctx); ok {
		addr = ip.Addr.String()
		glog.Infof("Login request from: %s", addr)
		span.Annotate([]otrace.Attribute{
			otrace.StringAttribute("client_ip", addr),
		}, "client ip for login")
	}

	user, err := s.authenticate(ctx, request)
	if err != nil {
		errMsg := fmt.Sprintf("authentication from address %s failed: %v", addr, err)
		glog.Errorf(errMsg)
		return nil, fmt.Errorf(errMsg)
	}

	resp := &api.Response{}
	accessJwt, err := getAccessJwt(request.Userid, user.Groups)
	if err != nil {
		errMsg := fmt.Sprintf("unable to get access jwt (userid=%s,addr=%s):%v",
			request.Userid, addr, err)
		glog.Errorf(errMsg)
		return nil, fmt.Errorf(errMsg)
	}
	refreshJwt, err := getRefreshJwt(request.Userid)
	if err != nil {
		errMsg := fmt.Sprintf("unable to get refresh jwt (userid=%s,addr=%s):%v",
			request.Userid, addr, err)
		glog.Errorf(errMsg)
		return nil, fmt.Errorf(errMsg)
	}

	loginJwt := api.Jwt{
		AccessJwt:  accessJwt,
		RefreshJwt: refreshJwt,
	}

	jwtBytes, err := loginJwt.Marshal()
	if err != nil {
		errMsg := fmt.Sprintf("unable to marshal jwt (userid=%s,addr=%s):%v",
			request.Userid, addr, err)
		glog.Errorf(errMsg)
		return nil, fmt.Errorf(errMsg)
	}
	resp.Json = jwtBytes
	return resp, nil
}

func (s *Server) authenticate(ctx context.Context, request *api.LoginRequest) (*acl.User, error) {
	if err := validateLoginRequest(request); err != nil {
		return nil, fmt.Errorf("invalid login request: %v", err)
	}

	var user *acl.User
	if len(request.RefreshToken) > 0 {
		user, err := authenticateToken(request.RefreshToken)
		if err != nil {
			return nil, fmt.Errorf("unable to authenticate the refresh token %v: %v",
				request.RefreshToken, err)
		}

		user, err = s.queryUser(ctx, user.UserID, "")
		if err != nil {
			return nil, fmt.Errorf("error while querying user with id: %v",
				request.Userid)
		}

		if user == nil {
			return nil, fmt.Errorf("user not found for id %v", request.Userid)
		}
	} else {
		var err error
		if ctx, err = appendAdminJwt(ctx); err != nil {
			return nil, fmt.Errorf("unable to append admin jwt:%v", err)
		}
		user, err = s.queryUser(ctx, request.Userid, request.Password)
		if err != nil {
			return nil, fmt.Errorf("error while querying user with id: %v",
				request.Userid)
		}

		if user == nil {
			return nil, fmt.Errorf("user not found for id %v", request.Userid)
		}
		if !user.PasswordMatch {
			return nil, fmt.Errorf("password mismatch for user: %v", request.Userid)
		}
	}

	return user, nil
}

func authenticateToken(refreshToken string) (*acl.User, error) {
	token, err := jwt.Parse(refreshToken, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return Config.HmacSecret, nil
	})

	if err != nil {
		return nil, fmt.Errorf("unable to parse refresh token:%v", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("claims in refresh token is not map claims:%v", refreshToken)
	}

	// by default, the MapClaims.Valid will return true if the exp field is not set
	// here we enforce the checking to make sure that the refresh token has not expired
	now := time.Now().Unix()
	if !claims.VerifyExpiresAt(now, true) {
		return nil, fmt.Errorf("refresh token has expired: %v", refreshToken)
	}

	userId, ok := claims["userid"].(string)
	if !ok {
		return nil, fmt.Errorf("userid in claims is not a string:%v", userId)
	}

	user := &acl.User{}
	user.UserID = userId
	groups, ok := claims["groups"].([]acl.Group)
	if ok {
		user.Groups = groups
	}
	return user, nil
}

func validateLoginRequest(request *api.LoginRequest) error {
	if request == nil {
		return fmt.Errorf("the request should not be nil")
	}
	// we will use the refresh token for authentication if it's set
	if len(request.RefreshToken) > 0 {
		return nil
	}

	// otherwise make sure both userid and password are set
	if len(request.Userid) == 0 {
		return fmt.Errorf("the userid should not be empty")
	}
	if len(request.Password) == 0 {
		return fmt.Errorf("the password should not be empty")
	}
	return nil
}

func getAccessJwt(userId string, groups []acl.Group) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"userid": userId,
		"groups": acl.GetGroupIDs(groups),
		// set the jwt exp according to the ttl
		"exp": json.Number(
			strconv.FormatInt(time.Now().Add(Config.AccessJwtTtl).Unix(), 10)),
	})

	jwtString, err := token.SignedString(Config.HmacSecret)
	if err != nil {
		return "", fmt.Errorf("unable to encode jwt to string: %v", err)
	}
	return jwtString, nil
}

func getRefreshJwt(userId string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"userid": userId,
		// set the jwt exp according to the ttl
		"exp": json.Number(
			strconv.FormatInt(time.Now().Add(Config.RefreshJwtTtl).Unix(), 10)),
	})

	jwtString, err := token.SignedString(Config.HmacSecret)
	if err != nil {
		return "", fmt.Errorf("unable to encode jwt to string: %v", err)
	}
	return jwtString, nil
}

const queryUser = `
    query search($userid: string, $password: string){
      user(func: eq(dgraph.xid, $userid)) {
	    uid
        password_match: checkpwd(dgraph.password, $password)
        dgraph.user.group {
          uid
          dgraph.xid
        }
      }
    }`

func (s *Server) queryUser(ctx context.Context, userid string, password string) (user *acl.User,
	err error) {
	queryVars := map[string]string{
		"$userid":   userid,
		"$password": password,
	}
	queryRequest := api.Request{
		Query: queryUser,
		Vars:  queryVars,
	}

	queryResp, err := s.Query(ctx, &queryRequest)
	if err != nil {
		glog.Errorf("Error while query user with id %s: %v", userid, err)
		return nil, err
	}
	user, err = acl.UnmarshalUser(queryResp, "user")
	if err != nil {
		return nil, err
	}
	return user, nil
}

func RetrieveAclsPeriodically(closeCh <-chan struct{}) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-closeCh:
			return
		case <-ticker.C:
			if err := (&Server{}).retrieveAcls(); err != nil {
				glog.Errorf("Error while retrieving acls:%v", err)
			}
		}
	}
}

const queryAcls = `
{
  allAcls(func: has(dgraph.group.acl)) {
    dgraph.xid
    dgraph.group.acl
  }
}
`

// the acl cache mapping group names to the group acl
var aclCache sync.Map

func InitAcl() {
	aclCache = sync.Map{}

	// upsert the admin account
	ctx, err := appendAdminJwt(context.Background())
	if err != nil {
		glog.Errorf("unable to append admin jwt")
		return
	}

	server := &Server{}
	adminUser, err := server.queryUser(ctx, "admin", "")
	if err != nil {
		glog.Errorf("error while querying the admin account")
		return
	}

	if adminUser != nil {
		// the admin user already exists, no need to create
		return
	}

	// insert the admin user
	createUserNQuads := []*api.NQuad{
		{
			Subject:     "_:newuser",
			Predicate:   "dgraph.xid",
			ObjectValue: &api.Value{Val: &api.Value_StrVal{StrVal: "admin"}},
		},
		{
			Subject:     "_:newuser",
			Predicate:   "dgraph.password",
			ObjectValue: &api.Value{Val: &api.Value_StrVal{StrVal: "password"}},
		}}

	mu := &api.Mutation{
		CommitNow: true,
		Set:       createUserNQuads,
	}

	if _, err := server.Mutate(ctx, mu); err != nil {
		glog.Errorf("unable to create user: %v", err)
		return
	}
	glog.Info("Created the admin account with the default password")
}

func appendAdminJwt(ctx context.Context) (context.Context, error) {
	// query the user with admin account
	adminJwt, err := getAccessJwt("admin", nil)
	if err != nil {
		return nil, fmt.Errorf("unable to get admin jwt:%v", err)
	}

	md := metadata.New(nil)
	md.Append("accessJwt", adminJwt)
	return metadata.NewIncomingContext(ctx, md), nil
}

func (s *Server) retrieveAcls() error {
	glog.Infof("Retrieving ACLs")
	queryRequest := api.Request{
		Query: queryAcls,
	}
	queryResp, err := s.Query(context.Background(), &queryRequest)
	if err != nil {
		return fmt.Errorf("unable to retrieve acls: %v", err)
	}
	groups, err := acl.UnmarshalGroups(queryResp.GetJson(), "allAcls")
	if err != nil {
		return err
	}

	storedEntries := 0
	for _, group := range groups {
		// convert the serialized acl into a map for easy lookups
		group.MappedAcls, err = acl.UnmarshalAcls([]byte(group.Acls))
		if err != nil {
			glog.Errorf("Error while unmarshalling ACLs for group %v:%v", group, err)
			continue
		}

		storedEntries++
		aclCache.Store(group.GroupID, &group)
	}
	glog.Infof("updated the ACL cache with %d entries", storedEntries)
	return nil
}

func (s *Server) AuthorizeQuery(ctx context.Context, parsedReq gql.Result) error {
	// extract the jwt and unmarshal the jwt to get the list of groups
	//glog.Infof("authorizing query with access jwt :%v", ctx.Value("accessJwt"))
	//accessJwt, ok := ctx.Value("accessJwt").(string)
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return fmt.Errorf("no metadata available")
	}
	glog.Infof("query metadata :%v", md)

	accessJwt := md.Get("accessJwt")
	if len(accessJwt) == 0 {
		//glog.Infof("no accessJwt available, type is %v", reflect.TypeOf(ctx.Value("accessJwt")))
		return fmt.Errorf("no accessJwt available")
	}
	aclUser, err := authenticateToken(accessJwt[0])
	if err != nil {
		return fmt.Errorf("token authentication failed:%v", err)
	}
	if aclUser.UserID == "admin" {
		// the admin account has access to everything
		return nil
	}
	glog.Infof("authorizing query with user id :%v", aclUser.UserID)
	// get the relevant predicates used in the query
	for _, query := range parsedReq.Query {
		if !s.authorizeGroups(aclUser.Groups, query.Attr, acl.Read) {
			return fmt.Errorf("unauthorized to access the predicate %v", query.Attr)
		}
	}
	return nil
}

// HasAccess returns true if any group is authorized to perform the operation on the predicate
func (s *Server) authorizeGroups(groups []acl.Group, predicate string, operation int32) bool {
	for _, group := range groups {
		if s.hasAccess(group, predicate, operation) {
			return true
		}
	}
	return false
}

// hasAccess checks the aclCache and returns if the specified group is authorized to perform the
// operation on the given predicate
func (s *Server) hasAccess(group acl.Group, predicate string, operation int32) bool {
	entry, found := aclCache.Load(group.GroupID)
	if !found {
		return false
	}
	aclGroup := entry.(*acl.Group)
	perm, found := aclGroup.MappedAcls[predicate]
	return found && (perm&operation) != 0
}
