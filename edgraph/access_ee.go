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

	"github.com/dgraph-io/badger/y"

	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/ee/acl"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/x"
	jwt "github.com/dgrijalva/jwt-go"
	"github.com/golang/glog"
	otrace "go.opencensus.io/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

func (s *Server) Login(ctx context.Context,
	request *api.LoginRequest) (*api.Response, error) {
	ctx, span := otrace.StartSpan(ctx, "server.Login")
	defer span.End()

	// record the client ip for this login request
	var addr string
	if ip, ok := peer.FromContext(ctx); ok {
		addr = ip.Addr.String()
		glog.Infof("login request from: %s", addr)
		span.Annotate([]otrace.Attribute{
			otrace.StringAttribute("client_ip", addr),
		}, "client ip for login")
	}

	user, err := s.authenticateLogin(ctx, request)
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

// authenticateLogin authenticates the login request using either the refresh token if present, or
// the <userId, password> pair. If authentication passes, it queries the user's uid and associated
// groups from DB and returns the user object
func (s *Server) authenticateLogin(ctx context.Context, request *api.LoginRequest) (*acl.User,
	error) {
	if err := validateLoginRequest(request); err != nil {
		return nil, fmt.Errorf("invalid login request: %v", err)
	}

	var user *acl.User
	if len(request.RefreshToken) > 0 {
		userData, err := validateToken(request.RefreshToken)
		if err != nil {
			return nil, fmt.Errorf("unable to authenticate the refresh token %v: %v",
				request.RefreshToken, err)
		}

		userId := userData[0]
		user, err = authorizeUser(ctx, userId, "")
		if err != nil {
			return nil, fmt.Errorf("error while querying user with id %v: %v", userId, err)
		}

		if user == nil {
			return nil, fmt.Errorf("unable to authenticate through refresh token: "+
				"user not found for id %v", userId)
		}

		glog.Infof("authenticated user %s through refresh token", userId)
		return user, nil
	}

	// authorize the user using password
	var err error
	user, err = authorizeUser(ctx, request.Userid, request.Password)
	if err != nil {
		return nil, fmt.Errorf("error while querying user with id %v: %v",
			request.Userid, err)
	}

	if user == nil {
		return nil, fmt.Errorf("unable to authenticate through password: "+
			"user not found for id %v", request.Userid)
	}
	if !user.PasswordMatch {
		return nil, fmt.Errorf("password mismatch for user: %v", request.Userid)
	}
	return user, nil
}

// validateToken verifies the signature and expiration of the jwt, and if validation passes,
// returns a slice of strings, where the first element is the extracted userId
// and the rest are groupIds encoded in the jwt.
func validateToken(jwtStr string) ([]string, error) {
	token, err := jwt.Parse(jwtStr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return Config.HmacSecret, nil
	})

	if err != nil {
		return nil, fmt.Errorf("unable to parse jwt token:%v", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("claims in jwt token is not map claims")
	}

	// by default, the MapClaims.Valid will return true if the exp field is not set
	// here we enforce the checking to make sure that the refresh token has not expired
	now := time.Now().Unix()
	if !claims.VerifyExpiresAt(now, true) {
		return nil, fmt.Errorf("Token is expired") // the same error msg that's used inside jwt-go
	}

	userId, ok := claims["userid"].(string)
	if !ok {
		return nil, fmt.Errorf("userid in claims is not a string:%v", userId)
	}

	groups, ok := claims["groups"].([]interface{})
	var groupIds []string
	if ok {
		groupIds = make([]string, 0, len(groups))
		for _, group := range groups {
			groupId, ok := group.(string)
			if !ok {
				// This shouldn't happen. So, no need to make the client try to refresh the tokens.
				return nil, fmt.Errorf("unable to convert group to string:%v", group)
			}

			groupIds = append(groupIds, groupId)
		}
	}
	return append([]string{userId}, groupIds...), nil
}

// validateLoginRequest validates that the login request has either the refresh token or the
// <user id, password> pair
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

// getAccessJwt constructs an access jwt with the given user id, groupIds,
// and expiration TTL specified by Config.AccessJwtTtl
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

// getRefreshJwt constructs a refresh jwt with the given user id, and expiration ttl specified by
// Config.RefreshJwtTtl
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

// authorizeUser queries the user with the given user id, and returns the associated uid,
// acl groups, and whether the password stored in DB matches the supplied password
func authorizeUser(ctx context.Context, userid string, password string) (*acl.User,
	error) {
	queryVars := map[string]string{
		"$userid":   userid,
		"$password": password,
	}
	queryRequest := api.Request{
		Query: queryUser,
		Vars:  queryVars,
	}

	queryResp, err := (&Server{}).doQuery(ctx, &queryRequest)
	if err != nil {
		glog.Errorf("Error while query user with id %s: %v", userid, err)
		return nil, err
	}
	user, err := acl.UnmarshalUser(queryResp, "user")
	if err != nil {
		return nil, err
	}
	return user, nil
}

func RefreshAcls(closer *y.Closer) {
	defer closer.Done()
	if len(Config.HmacSecret) == 0 {
		// the acl feature is not turned on
		return
	}

	ticker := time.NewTicker(Config.AclRefreshInterval)
	defer ticker.Stop()

	// retrieve the full data set of ACLs from the corresponding alpha server, and update the
	// aclCache
	retrieveAcls := func() error {
		glog.V(1).Infof("Refreshing ACLs")
		queryRequest := api.Request{
			Query: queryAcls,
		}

		ctx := context.Background()
		var err error
		queryResp, err := (&Server{}).doQuery(ctx, &queryRequest)
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
			group.MappedAcls, err = acl.UnmarshalAcl([]byte(group.Acls))
			if err != nil {
				glog.Errorf("Error while unmarshalling ACLs for group %v:%v", group, err)
				continue
			}

			storedEntries++
			aclCache.Store(group.GroupID, &group)
		}
		glog.V(1).Infof("Updated the ACL cache with %d entries", storedEntries)
		return nil
	}

	for {
		select {
		case <-closer.HasBeenClosed():
			return
		case <-ticker.C:
			if err := retrieveAcls(); err != nil {
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

// the acl cache mapping group names to the corresponding group acls
var aclCache sync.Map

// clear the aclCache and upsert the Groot account.
func ResetAcl() {
	if len(Config.HmacSecret) == 0 {
		// the acl feature is not turned on
		return
	}

	upsertGroot := func(ctx context.Context) error {
		queryVars := map[string]string{
			"$userid":   x.GrootId,
			"$password": "",
		}
		queryRequest := api.Request{
			Query: queryUser,
			Vars:  queryVars,
		}

		queryResp, err := (&Server{}).doQuery(ctx, &queryRequest)
		if err != nil {
			return fmt.Errorf("error while querying user with id %s: %v", x.GrootId, err)
		}
		startTs := queryResp.GetTxn().StartTs

		rootUser, err := acl.UnmarshalUser(queryResp, "user")
		if err != nil {
			return fmt.Errorf("error while unmarshaling the root user: %v", err)
		}
		if rootUser != nil {
			// the user already exists, no need to create
			return nil
		}

		// Insert Groot.
		createUserNQuads := []*api.NQuad{
			{
				Subject:     "_:newuser",
				Predicate:   "dgraph.xid",
				ObjectValue: &api.Value{Val: &api.Value_StrVal{StrVal: x.GrootId}},
			},
			{
				Subject:     "_:newuser",
				Predicate:   "dgraph.password",
				ObjectValue: &api.Value{Val: &api.Value_StrVal{StrVal: "password"}},
			}}

		mu := &api.Mutation{
			StartTs:   startTs,
			CommitNow: true,
			Set:       createUserNQuads,
		}

		if _, err := (&Server{}).doMutate(context.Background(), mu); err != nil {
			return err
		}
		return nil
	}

	aclCache = sync.Map{}
	for {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if err := upsertGroot(ctx); err != nil {
			glog.Infof("Unable to upsert the groot account. Error: %v", err)
			time.Sleep(100 * time.Millisecond)
		} else {
			return
		}
	}
}

// extract the userId, groupIds from the accessJwt in the context
func extractUserAndGroups(ctx context.Context) ([]string, error) {
	// extract the jwt and unmarshal the jwt to get the list of groups
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, fmt.Errorf("no metadata available")
	}
	accessJwt := md.Get("accessJwt")
	if len(accessJwt) == 0 {
		return nil, fmt.Errorf("no accessJwt available")
	}

	return validateToken(accessJwt[0])
}

//authorizeAlter parses the Schema in the operation and authorizes the operation using the aclCache
func authorizeAlter(ctx context.Context, op *api.Operation) error {
	if len(Config.HmacSecret) == 0 {
		// the user has not turned on the acl feature
		return nil
	}

	userData, err := extractUserAndGroups(ctx)
	if err != nil {
		return status.Error(codes.Unauthenticated, err.Error())
	}
	if isGroot(userData) {
		return nil
	}

	// if we get here, we know the user is not Groot.
	if op.DropAll {
		return fmt.Errorf("only Groot is allowed to drop all data")
	}

	groupIds := userData[1:]
	if len(op.DropAttr) > 0 {
		// check that we have the modify permission on the predicate
		if err := authorizePredicate(groupIds, op.DropAttr, acl.Modify); err != nil {
			return fmt.Errorf("unauthorized to modify the predicate:%v", err)
		}
		return nil
	}

	updates, err := schema.Parse(op.Schema)
	if err != nil {
		return err
	}
	for _, update := range updates {
		if err := authorizePredicate(groupIds, update.Predicate, acl.Modify); err != nil {
			return fmt.Errorf("unauthorized to modify the predicate: %v", err)
		}
	}
	return nil
}

// parsePredsFromMutation returns a union set of all the predicate names in the input nquads
func parsePredsFromMutation(nquads []*api.NQuad) map[string]struct{} {
	preds := make(map[string]struct{})
	for _, nquad := range nquads {
		preds[nquad.Predicate] = struct{}{}
	}
	return preds
}

// authorizeMutation authorizes the mutation using the aclCache
func authorizeMutation(ctx context.Context, mu *api.Mutation) error {
	if len(Config.HmacSecret) == 0 {
		// the user has not turned on the acl feature
		return nil
	}

	userData, err := extractUserAndGroups(ctx)
	if err != nil {
		return status.Error(codes.Unauthenticated, err.Error())
	}
	if isGroot(userData) {
		// Groot has access to everything.
		return nil
	}

	gmu, err := parseMutationObject(mu)
	if err != nil {
		return err
	}

	groupIds := userData[1:]
	for pred := range parsePredsFromMutation(gmu.Set) {
		if err := authorizePredicate(groupIds, pred, acl.Write); err != nil {
			return fmt.Errorf("unauthorized to access the predicate: %v", err)
		}
	}
	return nil
}

func parsePredsFromQuery(gqls []*gql.GraphQuery) map[string]struct{} {
	preds := make(map[string]struct{})
	for _, gq := range gqls {

		if gq.Func != nil {
			preds[gq.Func.Attr] = struct{}{}
		}

		if len(gq.Attr) > 0 {
			preds[gq.Attr] = struct{}{}
		}

		for childPred := range parsePredsFromQuery(gq.Children) {
			preds[childPred] = struct{}{}
		}
	}
	return preds
}

func isGroot(userData []string) bool {
	if len(userData) == 0 {
		return false
	}

	return userData[0] == x.GrootId
}

//authorizeQuery authorizes the query using the aclCache
func authorizeQuery(ctx context.Context, req *api.Request) error {
	if len(Config.HmacSecret) == 0 {
		// the user has not turned on the acl feature
		return nil
	}

	userData, err := extractUserAndGroups(ctx)
	if err != nil {
		return status.Error(codes.Unauthenticated, err.Error())
	}
	if isGroot(userData) {
		return nil
	}

	parsedReq, err := gql.Parse(gql.Request{
		Str:       req.Query,
		Variables: req.Vars,
	})
	if err != nil {
		return err
	}

	groupIds := userData[1:]
	for pred := range parsePredsFromQuery(parsedReq.Query) {
		if err := authorizePredicate(groupIds, pred, acl.Read); err != nil {
			return status.Error(codes.PermissionDenied,
				fmt.Sprintf("unauthorized to access the predicate: %v", err))
		}
	}
	return nil
}

func authorizePredicate(groups []string, predicate string, operation *acl.Operation) error {
	for _, group := range groups {
		if err := hasAccess(group, predicate, operation); err == nil {
			return nil
		}
	}
	return fmt.Errorf("unauthorized to do %s on predicate %s", operation.Name, predicate)
}

// hasAccess checks the aclCache and returns whether the specified group is authorized to perform
// the operation on the given predicate
func hasAccess(groupId string, predicate string, operation *acl.Operation) error {
	entry, found := aclCache.Load(groupId)
	if !found {
		return fmt.Errorf("acl not found for group %v", groupId)
	}
	aclGroup := entry.(*acl.Group)
	perm, found := aclGroup.MappedAcls[predicate]
	allowed := found && (perm&operation.Code) != 0
	glog.V(1).Infof("Authorizing group %v on predicate %v for %s, allowed %v", groupId,
		predicate, operation.Name, allowed)
	if !allowed {
		return fmt.Errorf("group %s not allowed to do %s on predicate %s",
			groupId, operation.Name, predicate)
	}
	return nil
}
