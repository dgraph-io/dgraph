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
	"strings"
	"time"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/ristretto/z"

	"github.com/dgraph-io/dgraph/query"

	"github.com/pkg/errors"

	bpb "github.com/dgraph-io/badger/v3/pb"
	"github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/dgraph/ee/acl"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
	jwt "github.com/dgrijalva/jwt-go"
	"github.com/golang/glog"
	otrace "go.opencensus.io/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type predsAndvars struct {
	preds []string
	vars  map[string]string
}

// Login handles login requests from clients.
func (s *Server) Login(ctx context.Context,
	request *api.LoginRequest) (*api.Response, error) {

	if err := x.HealthCheck(); err != nil {
		return nil, err
	}

	if !worker.EnterpriseEnabled() {
		return nil, errors.New("Enterprise features are disabled. You can enable them by " +
			"supplying the appropriate license file to Dgraph Zero using the HTTP endpoint.")
	}

	ctx, span := otrace.StartSpan(ctx, "server.Login")
	defer span.End()

	// record the client ip for this login request
	var addr string
	if ipAddr, err := hasAdminAuth(ctx, "Login"); err != nil {
		return nil, err
	} else {
		addr = ipAddr.String()
		span.Annotate([]otrace.Attribute{
			otrace.StringAttribute("client_ip", addr),
		}, "client ip for login")
	}

	user, err := s.authenticateLogin(ctx, request)
	if err != nil {
		glog.Errorf("Authentication from address %s failed: %v", addr, err)
		return nil, x.ErrorInvalidLogin
	}
	glog.Infof("%s logged in successfully", user.UserID)

	resp := &api.Response{}
	accessJwt, err := getAccessJwt(user.UserID, user.Groups, request.Namespace)
	if err != nil {
		errMsg := fmt.Sprintf("unable to get access jwt (userid=%s,addr=%s):%v",
			user.UserID, addr, err)
		glog.Errorf(errMsg)
		return nil, errors.Errorf(errMsg)
	}

	refreshJwt, err := getRefreshJwt(user.UserID, request.Namespace)
	if err != nil {
		errMsg := fmt.Sprintf("unable to get refresh jwt (userid=%s,addr=%s):%v",
			user.UserID, addr, err)
		glog.Errorf(errMsg)
		return nil, errors.Errorf(errMsg)
	}

	loginJwt := api.Jwt{
		AccessJwt:  accessJwt,
		RefreshJwt: refreshJwt,
	}

	jwtBytes, err := loginJwt.Marshal()
	if err != nil {
		errMsg := fmt.Sprintf("unable to marshal jwt (userid=%s,addr=%s):%v",
			user.UserID, addr, err)
		glog.Errorf(errMsg)
		return nil, errors.Errorf(errMsg)
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
		return nil, errors.Wrapf(err, "invalid login request")
	}

	// In case of login, we can't extract namespace from JWT because we have not yet given JWT
	// to the user, so the login request should contain the namespace, which is then set to ctx.
	ctx = x.AttachNamespace(ctx, request.Namespace)
	var user *acl.User
	if len(request.RefreshToken) > 0 {
		userData, err := validateToken(request.RefreshToken)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to authenticate the refresh token %v",
				request.RefreshToken)
		}

		userId := userData[0]
		user, err = authorizeUser(ctx, userId, "")
		if err != nil {
			return nil, errors.Wrapf(err, "while querying user with id %v", userId)
		}

		if user == nil {
			return nil, errors.Errorf("unable to authenticate: invalid credentials")
		}

		glog.Infof("Authenticated user %s through refresh token", userId)
		return user, nil
	}

	// authorize the user using password
	var err error
	user, err = authorizeUser(ctx, request.Userid, request.Password)
	if err != nil {
		return nil, errors.Wrapf(err, "while querying user with id %v",
			request.Userid)
	}

	if user == nil {
		return nil, errors.Errorf("unable to authenticate: invalid credentials")
	}
	if !user.PasswordMatch {
		return nil, x.ErrorInvalidLogin
	}
	return user, nil
}

// validateToken verifies the signature and expiration of the jwt, and if validation passes,
// returns a slice of strings, where the first element is the extracted userId
// and the rest are groupIds encoded in the jwt.
func validateToken(jwtStr string) ([]string, error) {
	claims, err := x.ParseJWT(jwtStr)
	if err != nil {
		return nil, err
	}
	// by default, the MapClaims.Valid will return true if the exp field is not set
	// here we enforce the checking to make sure that the refresh token has not expired
	now := time.Now().Unix()
	if !claims.VerifyExpiresAt(now, true) {
		return nil, errors.Errorf("Token is expired") // the same error msg that's used inside jwt-go
	}

	userId, ok := claims["userid"].(string)
	if !ok {
		return nil, errors.Errorf("userid in claims is not a string:%v", userId)
	}

	namespace, ok := claims["namespace"].(float64)
	if !ok {
		return nil, errors.Errorf("namespace in claims is not valid:%v", namespace)
	}

	groups, ok := claims["groups"].([]interface{})
	var groupIds []string
	if ok {
		groupIds = make([]string, 0, len(groups))
		for _, group := range groups {
			groupId, ok := group.(string)
			if !ok {
				// This shouldn't happen. So, no need to make the client try to refresh the tokens.
				return nil, errors.Errorf("unable to convert group to string:%v", group)
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
		return errors.Errorf("the request should not be nil")
	}
	// we will use the refresh token for authentication if it's set
	if len(request.RefreshToken) > 0 {
		return nil
	}

	// otherwise make sure both userid and password are set
	if len(request.Userid) == 0 {
		return errors.Errorf("the userid should not be empty")
	}
	if len(request.Password) == 0 {
		return errors.Errorf("the password should not be empty")
	}
	return nil
}

// getAccessJwt constructs an access jwt with the given user id, groupIds, namespace
// and expiration TTL specified by worker.Config.AccessJwtTtl
func getAccessJwt(userId string, groups []acl.Group, namespace uint64) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"userid":    userId,
		"groups":    acl.GetGroupIDs(groups),
		"namespace": namespace,
		// set the jwt exp according to the ttl
		"exp": time.Now().Add(worker.Config.AccessJwtTtl).Unix(),
	})

	jwtString, err := token.SignedString([]byte(worker.Config.HmacSecret))
	if err != nil {
		return "", errors.Errorf("unable to encode jwt to string: %v", err)
	}
	return jwtString, nil
}

// getRefreshJwt constructs a refresh jwt with the given user id, namespace and expiration ttl
// specified by worker.Config.RefreshJwtTtl
func getRefreshJwt(userId string, namespace uint64) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"userid":    userId,
		"namespace": namespace,
		"exp":       time.Now().Add(worker.Config.RefreshJwtTtl).Unix(),
	})

	jwtString, err := token.SignedString([]byte(worker.Config.HmacSecret))
	if err != nil {
		return "", errors.Errorf("unable to encode jwt to string: %v", err)
	}
	return jwtString, nil
}

const queryUser = `
    query search($userid: string, $password: string){
      user(func: eq(dgraph.xid, $userid)) {
	    uid
        dgraph.xid
        password_match: checkpwd(dgraph.password, $password)
        dgraph.user.group {
          uid
          dgraph.xid
        }
      }
    }`

// authorizeUser queries the user with the given user id, and returns the associated uid,
// acl groups, and whether the password stored in DB matches the supplied password
func authorizeUser(ctx context.Context, userid string, password string) (
	*acl.User, error) {

	queryVars := map[string]string{
		"$userid":   userid,
		"$password": password,
	}
	req := &Request{
		req: &api.Request{
			Query: queryUser,
			Vars:  queryVars,
		},
		doAuth: NoAuthorize,
	}
	queryResp, err := (&Server{}).doQuery(ctx, req)
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

// RefreshAcls queries for the ACL triples and refreshes the ACLs accordingly.
func RefreshAcls(closer *z.Closer) {
	defer func() {
		glog.Infoln("RefreshAcls closed")
		closer.Done()
	}()
	if len(worker.Config.HmacSecret) == 0 {
		// the acl feature is not turned on
		return
	}

	// retrieve the full data set of ACLs from the corresponding alpha server, and update the
	// aclCachePtr
	var maxRefreshTs uint64
	retrieveAcls := func(ns uint64, refreshTs uint64) error {
		if refreshTs <= maxRefreshTs {
			return nil
		}
		maxRefreshTs = refreshTs

		glog.V(3).Infof("Refreshing ACLs")
		req := &Request{
			req: &api.Request{
				Query:    queryAcls,
				ReadOnly: true,
				StartTs:  refreshTs,
			},
			doAuth: NoAuthorize,
		}

		ctx := x.AttachNamespace(closer.Ctx(), ns)
		queryResp, err := (&Server{}).doQuery(ctx, req)
		if err != nil {
			return errors.Errorf("unable to retrieve acls: %v", err)
		}
		groups, err := acl.UnmarshalGroups(queryResp.GetJson(), "allAcls")
		if err != nil {
			return err
		}

		aclCachePtr.update(ns, groups)
		glog.V(3).Infof("Updated the ACL cache")
		return nil
	}

	closer.AddRunning(1)
	go worker.SubscribeForUpdates(aclPrefixes, x.IgnoreBytes, func(kvs *bpb.KVList) {
		if kvs == nil || len(kvs.Kv) == 0 {
			return
		}
		kv := x.KvWithMaxVersion(kvs, aclPrefixes)
		pk, err := x.Parse(kv.GetKey())
		if err != nil {
			glog.Fatalf("Got a key from subscription which is not parsable: %s", err)
		}
		glog.V(3).Infof("Got ACL update via subscription for attr: %s", pk.Attr)

		ns, _ := x.ParseNamespaceAttr(pk.Attr)
		if err := retrieveAcls(ns, kv.GetVersion()); err != nil {
			glog.Errorf("Error while retrieving acls: %v", err)
		}
	}, 1, closer)

	<-closer.HasBeenClosed()
}

const queryAcls = `
{
  allAcls(func: type(dgraph.type.Group)) {
    dgraph.xid
	dgraph.acl.rule {
		dgraph.rule.predicate
		dgraph.rule.permission
	}
	~dgraph.user.group{
		dgraph.xid
	}
  }
}
`

var aclPrefixes = [][]byte{
	x.PredicatePrefix(x.GalaxyAttr("dgraph.acl.permission")),
	x.PredicatePrefix(x.GalaxyAttr("dgraph.acl.predicate")),
	x.PredicatePrefix(x.GalaxyAttr("dgraph.acl.rule")),
	x.PredicatePrefix(x.GalaxyAttr("dgraph.user.group")),
	x.PredicatePrefix(x.GalaxyAttr("dgraph.type.Group")),
	x.PredicatePrefix(x.GalaxyAttr("dgraph.xid")),
}

// clears the aclCachePtr and upserts the Groot account.
func ResetAcl(closer *z.Closer) {
	defer func() {
		glog.Infof("ResetAcl closed")
		closer.Done()
	}()

	if len(worker.Config.HmacSecret) == 0 {
		// The acl feature is not turned on.
		return
	}
	for closer.Ctx().Err() == nil {
		ctx, cancel := context.WithTimeout(closer.Ctx(), time.Minute)
		defer cancel()
		ctx = x.AttachNamespace(ctx, x.GalaxyNamespace)
		if err := upsertGuardian(ctx); err != nil {
			glog.Infof("Unable to upsert the guardian group. Error: %v", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}
		break
	}

	for closer.Ctx().Err() == nil {
		ctx, cancel := context.WithTimeout(closer.Ctx(), time.Minute)
		defer cancel()
		ctx = x.AttachNamespace(ctx, x.GalaxyNamespace)
		if err := upsertGroot(ctx, "password"); err != nil {
			glog.Infof("Unable to upsert the groot account. Error: %v", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}
		break
	}
}

// upsertGuardian must be called after setting the namespace in the context.
func upsertGuardian(ctx context.Context) error {
	query := fmt.Sprintf(`
			{
				guid as guardians(func: eq(dgraph.xid, "%s")){
					uid
				}
			}
		`, x.GuardiansId)
	groupNQuads := acl.CreateGroupNQuads(x.GuardiansId)
	req := &Request{
		req: &api.Request{
			CommitNow: true,
			Query:     query,
			Mutations: []*api.Mutation{
				{
					Set:  groupNQuads,
					Cond: "@if(eq(len(guid), 0))",
				},
			},
		},
		doAuth: NoAuthorize,
	}

	resp, err := (&Server{}).doQuery(ctx, req)

	// Structs to parse guardians group uid from query response
	type groupNode struct {
		Uid string `json:"uid"`
	}

	type groupQryResp struct {
		GuardiansGroup []groupNode `json:"guardians"`
	}

	if err != nil {
		return errors.Wrapf(err, "while upserting group with id %s", x.GuardiansId)
	}
	var groupResp groupQryResp
	var guardiansUidStr string
	if err := json.Unmarshal(resp.GetJson(), &groupResp); err != nil {
		return errors.Wrap(err, "Couldn't unmarshal response from guardians group query")
	}

	if len(groupResp.GuardiansGroup) == 0 {
		// no guardians group found
		// Extract guardians group uid from mutation
		newGroupUidMap := resp.GetUids()
		guardiansUidStr = newGroupUidMap["newgroup"]
	} else if len(groupResp.GuardiansGroup) == 1 {
		// we found a guardians group
		guardiansUidStr = groupResp.GuardiansGroup[0].Uid
	} else {
		return errors.Wrap(err, "Multiple guardians group found")
	}

	uid, err := strconv.ParseUint(guardiansUidStr, 0, 64)
	if err != nil {
		return errors.Wrapf(err, "Error while parsing Uid: %s of guardians Group", guardiansUidStr)
	}
	ns, err := x.ExtractNamespace(ctx)
	if err != nil {
		return errors.Wrapf(err, "While upserting group with id %s", x.GuardiansId)
	}
	x.GuardiansUid.Store(ns, uid)
	glog.V(2).Infof("Successfully upserted the guardian of namespace: %d\n", ns)
	return nil
}

// upsertGroot must be called after setting the namespace in the context.
func upsertGroot(ctx context.Context, passwd string) error {
	// groot is the default user of guardians group.
	query := fmt.Sprintf(`
			{
				grootid as grootUser(func: eq(dgraph.xid, "%s")){
					uid
				}
				guid as var(func: eq(dgraph.xid, "%s"))
			}
		`, x.GrootId, x.GuardiansId)
	userNQuads := acl.CreateUserNQuads(x.GrootId, passwd)
	userNQuads = append(userNQuads, &api.NQuad{
		Subject:   "_:newuser",
		Predicate: "dgraph.user.group",
		ObjectId:  "uid(guid)",
	})
	req := &Request{
		req: &api.Request{
			CommitNow: true,
			Query:     query,
			Mutations: []*api.Mutation{
				{
					Set: userNQuads,
					// Assuming that if groot exists, it is in guardian group
					Cond: "@if(eq(len(grootid), 0) and gt(len(guid), 0))",
				},
			},
		},
		doAuth: NoAuthorize,
	}

	resp, err := (&Server{}).doQuery(ctx, req)
	if err != nil {
		return errors.Wrapf(err, "while upserting user with id %s", x.GrootId)
	}

	// Structs to parse groot user uid from query response
	type userNode struct {
		Uid string `json:"uid"`
	}

	type userQryResp struct {
		GrootUser []userNode `json:"grootUser"`
	}

	var grootUserUid string
	var userResp userQryResp
	if err := json.Unmarshal(resp.GetJson(), &userResp); err != nil {
		return errors.Wrap(err, "Couldn't unmarshal response from groot user query")
	}
	if len(userResp.GrootUser) == 0 {
		// no groot user found from query
		// Extract uid of created groot user from mutation
		newUserUidMap := resp.GetUids()
		grootUserUid = newUserUidMap["newuser"]
	} else if len(userResp.GrootUser) == 1 {
		// we found a groot user
		grootUserUid = userResp.GrootUser[0].Uid
	} else {
		return errors.Wrap(err, "Multiple groot users found")
	}

	uid, err := strconv.ParseUint(grootUserUid, 0, 64)
	if err != nil {
		return errors.Wrapf(err, "Error while parsing Uid: %s of groot user", grootUserUid)
	}
	ns, err := x.ExtractNamespace(ctx)
	if err != nil {
		return errors.Wrapf(err, "While upserting user with id %s", x.GrootId)
	}
	x.GrootUid.Store(ns, uid)
	glog.V(2).Infof("Successfully upserted groot account for namespace %d\n", ns)
	return nil
}

// extract the userId, groupIds from the accessJwt in the context
func extractUserAndGroups(ctx context.Context) ([]string, error) {
	accessJwt, err := x.ExtractJwt(ctx)
	if err != nil {
		return nil, err
	}
	return validateToken(accessJwt[0])
}

type authPredResult struct {
	allowed []string
	blocked map[string]struct{}
}

func authorizePreds(ctx context.Context, userData, preds []string,
	aclOp *acl.Operation) (*authPredResult, error) {

	ns, err := x.ExtractNamespace(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "While authorizing preds")
	}
	userId := userData[0]
	groupIds := userData[1:]
	blockedPreds := make(map[string]struct{})
	for _, pred := range preds {
		nsPred := x.NamespaceAttr(ns, pred)
		if err := aclCachePtr.authorizePredicate(groupIds, nsPred, aclOp); err != nil {
			logAccess(&accessEntry{
				userId:    userId,
				groups:    groupIds,
				preds:     preds,
				operation: aclOp,
				allowed:   false,
			})

			blockedPreds[pred] = struct{}{}
		}
	}
	aclCachePtr.RLock()
	allowedPreds := make([]string, len(aclCachePtr.userPredPerms[userId]))
	// User can have multiple permission for same predicate, add predicate
	// only if the acl.Op is covered in the set of permissions for the user
	for predicate, perm := range aclCachePtr.userPredPerms[userId] {
		if (perm & aclOp.Code) > 0 {
			allowedPreds = append(allowedPreds, predicate)
		}
	}
	aclCachePtr.RUnlock()
	return &authPredResult{allowed: allowedPreds, blocked: blockedPreds}, nil
}

// authorizeAlter parses the Schema in the operation and authorizes the operation
// using the aclCachePtr. It will return error if any one of the predicates specified in alter
// are not authorized.
func authorizeAlter(ctx context.Context, op *api.Operation) error {
	if len(worker.Config.HmacSecret) == 0 {
		// the user has not turned on the acl feature
		return nil
	}

	// extract the list of predicates from the operation object
	var preds []string
	switch {
	case len(op.DropAttr) > 0:
		preds = []string{op.DropAttr}
	case op.DropOp == api.Operation_ATTR && len(op.DropValue) > 0:
		preds = []string{op.DropValue}
	default:
		update, err := schema.Parse(op.Schema)
		if err != nil {
			return err
		}

		for _, u := range update.Preds {
			preds = append(preds, x.ParseAttr(u.Predicate))
		}
	}
	var userId string
	var groupIds []string

	// doAuthorizeAlter checks if alter of all the predicates are allowed
	// as a byproduct, it also sets the userId, groups variables
	doAuthorizeAlter := func() error {
		userData, err := extractUserAndGroups(ctx)
		if err != nil {
			// We don't follow fail open approach anymore.
			return status.Error(codes.Unauthenticated, err.Error())
		}

		userId = userData[0]
		groupIds = userData[1:]

		if x.IsGuardian(groupIds) {
			// Members of guardian group are allowed to alter anything.
			return nil
		}

		// if we get here, we know the user is not a guardian.
		if isDropAll(op) || op.DropOp == api.Operation_DATA {
			return errors.Errorf(
				"only guardians are allowed to drop all data, but the current user is %s", userId)
		}

		result, err := authorizePreds(ctx, userData, preds, acl.Modify)
		if err != nil {
			return nil
		}
		if len(result.blocked) > 0 {
			var msg strings.Builder
			for key := range result.blocked {
				x.Check2(msg.WriteString(key))
				x.Check2(msg.WriteString(" "))
			}
			return status.Errorf(codes.PermissionDenied,
				"unauthorized to alter following predicates: %s\n", msg.String())
		}
		return nil
	}

	err := doAuthorizeAlter()
	span := otrace.FromContext(ctx)
	if span != nil {
		span.Annotatef(nil, (&accessEntry{
			userId:    userId,
			groups:    groupIds,
			preds:     preds,
			operation: acl.Modify,
			allowed:   err == nil,
		}).String())
	}

	return err
}

// parsePredsFromMutation returns a union set of all the predicate names in the input nquads
func parsePredsFromMutation(nquads []*api.NQuad) []string {
	// use a map to dedup predicates
	predsMap := make(map[string]struct{})
	for _, nquad := range nquads {
		// _STAR_ALL is not a predicate in itself.
		if nquad.Predicate != "_STAR_ALL" {
			predsMap[nquad.Predicate] = struct{}{}
		}
	}

	preds := make([]string, 0, len(predsMap))
	for pred := range predsMap {
		preds = append(preds, pred)
	}

	return preds
}

func isAclPredMutation(nquads []*api.NQuad) bool {
	for _, nquad := range nquads {
		if nquad.Predicate == "dgraph.group.acl" && nquad.ObjectValue != nil {
			// this mutation is trying to change the permission of some predicate
			// check if the predicate list contains an ACL predicate
			if _, ok := nquad.ObjectValue.Val.(*api.Value_BytesVal); ok {
				aclBytes := nquad.ObjectValue.Val.(*api.Value_BytesVal)
				var aclsToChange []acl.Acl
				err := json.Unmarshal(aclBytes.BytesVal, &aclsToChange)
				if err != nil {
					glog.Errorf(fmt.Sprintf("Unable to unmarshal bytes under the dgraph.group.acl "+
						"predicate:	%v", err))
					continue
				}
				for _, aclToChange := range aclsToChange {
					if x.IsAclPredicate(aclToChange.Predicate) {
						return true
					}
				}
			}
		}
	}
	return false
}

// authorizeMutation authorizes the mutation using the aclCachePtr. It will return permission
// denied error if any one of the predicates in mutation(set or delete) is unauthorized.
// At this stage, namespace is not attached in the predicates.
func authorizeMutation(ctx context.Context, gmu *gql.Mutation) error {
	if len(worker.Config.HmacSecret) == 0 {
		// the user has not turned on the acl feature
		return nil
	}

	preds := parsePredsFromMutation(gmu.Set)
	// Del predicates weren't included before.
	// A bug probably since f115de2eb6a40d882a86c64da68bf5c2a33ef69a
	preds = append(preds, parsePredsFromMutation(gmu.Del)...)

	var userId string
	var groupIds []string
	// doAuthorizeMutation checks if modification of all the predicates are allowed
	// as a byproduct, it also sets the userId and groups
	doAuthorizeMutation := func() error {
		userData, err := extractUserAndGroups(ctx)
		if err != nil {
			// We don't follow fail open approach anymore.
			return status.Error(codes.Unauthenticated, err.Error())
		}

		userId = userData[0]
		groupIds = userData[1:]

		if x.IsGuardian(groupIds) {
			// Members of guardians group are allowed to mutate anything
			// (including delete) except the permission of the acl predicates.
			switch {
			case isAclPredMutation(gmu.Set):
				return errors.Errorf("the permission of ACL predicates can not be changed")
			case isAclPredMutation(gmu.Del):
				return errors.Errorf("ACL predicates can't be deleted")
			}
			return nil
		}
		result, err := authorizePreds(ctx, userData, preds, acl.Write)
		if err != nil {
			return err
		}
		if len(result.blocked) > 0 {
			var msg strings.Builder
			for key := range result.blocked {
				x.Check2(msg.WriteString(key))
				x.Check2(msg.WriteString(" "))
			}
			return status.Errorf(codes.PermissionDenied,
				"unauthorized to mutate following predicates: %s\n", msg.String())
		}
		gmu.AllowedPreds = result.allowed
		return nil
	}

	err := doAuthorizeMutation()

	span := otrace.FromContext(ctx)
	if span != nil {
		span.Annotatef(nil, (&accessEntry{
			userId:    userId,
			groups:    groupIds,
			preds:     preds,
			operation: acl.Write,
			allowed:   err == nil,
		}).String())
	}

	return err
}

func parsePredsFromQuery(gqls []*gql.GraphQuery) predsAndvars {
	predsMap := make(map[string]struct{})
	varsMap := make(map[string]string)
	for _, gq := range gqls {
		if gq.Func != nil {
			predsMap[gq.Func.Attr] = struct{}{}
		}
		if len(gq.Var) > 0 {
			varsMap[gq.Var] = gq.Attr
		}
		if len(gq.Attr) > 0 && gq.Attr != "uid" && gq.Attr != "expand" && gq.Attr != "val" {
			predsMap[gq.Attr] = struct{}{}

		}
		for _, ord := range gq.Order {
			predsMap[ord.Attr] = struct{}{}
		}
		for _, gbAttr := range gq.GroupbyAttrs {
			predsMap[gbAttr.Attr] = struct{}{}
		}
		for _, pred := range parsePredsFromFilter(gq.Filter) {
			predsMap[pred] = struct{}{}
		}
		childPredandVars := parsePredsFromQuery(gq.Children)
		for _, childPred := range childPredandVars.preds {
			predsMap[childPred] = struct{}{}
		}
		for childVar := range childPredandVars.vars {
			varsMap[childVar] = childPredandVars.vars[childVar]
		}
	}
	preds := make([]string, 0, len(predsMap))
	for pred := range predsMap {
		if len(pred) > 0 {
			if _, found := varsMap[pred]; !found {
				preds = append(preds, pred)
			}
		}
	}

	pv := predsAndvars{preds: preds, vars: varsMap}
	return pv
}

func parsePredsFromFilter(f *gql.FilterTree) []string {
	var preds []string
	if f == nil {
		return preds
	}
	if f.Func != nil && len(f.Func.Attr) > 0 {
		preds = append(preds, f.Func.Attr)
	}
	for _, ch := range f.Child {
		preds = append(preds, parsePredsFromFilter(ch)...)
	}
	return preds
}

type accessEntry struct {
	userId    string
	groups    []string
	preds     []string
	operation *acl.Operation
	allowed   bool
}

func (log *accessEntry) String() string {
	return fmt.Sprintf("ACL-LOG Authorizing user %q with groups %q on predicates %q "+
		"for %q, allowed:%v", log.userId, strings.Join(log.groups, ","),
		strings.Join(log.preds, ","), log.operation.Name, log.allowed)
}

func logAccess(log *accessEntry) {
	if glog.V(1) {
		glog.Info(log.String())
	}
}

//authorizeQuery authorizes the query using the aclCachePtr. It will silently drop all
// unauthorized predicates from query.
// At this stage, namespace is not attached in the predicates.
func authorizeQuery(ctx context.Context, parsedReq *gql.Result, graphql bool) error {
	if len(worker.Config.HmacSecret) == 0 {
		// the user has not turned on the acl feature
		return nil
	}

	var userId string
	var groupIds []string
	predsAndvars := parsePredsFromQuery(parsedReq.Query)
	preds := predsAndvars.preds
	varsToPredMap := predsAndvars.vars

	// Need this to efficiently identify blocked variables from the
	// list of blocked predicates
	predToVarsMap := make(map[string]string)
	for k, v := range varsToPredMap {
		predToVarsMap[v] = k
	}

	doAuthorizeQuery := func() (map[string]struct{}, []string, error) {
		userData, err := extractUserAndGroups(ctx)
		if err != nil {
			return nil, nil, status.Error(codes.Unauthenticated, err.Error())
		}

		userId = userData[0]
		groupIds = userData[1:]

		if x.IsGuardian(groupIds) {
			// Members of guardian groups are allowed to query anything.
			return nil, nil, nil
		}

		result, err := authorizePreds(ctx, userData, preds, acl.Read)
		return result.blocked, result.allowed, err
	}

	blockedPreds, allowedPreds, err := doAuthorizeQuery()
	if err != nil {
		return err
	}

	if span := otrace.FromContext(ctx); span != nil {
		span.Annotatef(nil, (&accessEntry{
			userId:    userId,
			groups:    groupIds,
			preds:     preds,
			operation: acl.Read,
			allowed:   err == nil,
		}).String())
	}

	if len(blockedPreds) != 0 {
		// For GraphQL requests, we allow filtered access to the ACL predicates.
		// Filter for user_id and group_id is applied for the currently logged in user.
		if graphql {
			for _, gq := range parsedReq.Query {
				addUserFilterToQuery(gq, userId, groupIds)
			}
			// blockedPreds might have acl predicates which we want to allow access through
			// graphql, so deleting those from here.
			for _, pred := range x.AllACLPredicates() {
				delete(blockedPreds, pred)
			}
			// In query context ~predicate and predicate are considered different.
			delete(blockedPreds, "~dgraph.user.group")
		}

		blockedVars := make(map[string]struct{})
		for predicate := range blockedPreds {
			if variable, found := predToVarsMap[predicate]; found {
				// Add variables to blockedPreds to delete from Query
				blockedPreds[variable] = struct{}{}
				// Collect blocked Variables to remove from QueryVars
				blockedVars[variable] = struct{}{}
			}
		}
		parsedReq.Query = removePredsFromQuery(parsedReq.Query, blockedPreds)
		parsedReq.QueryVars = removeVarsFromQueryVars(parsedReq.QueryVars, blockedVars)
	}
	for i := range parsedReq.Query {
		parsedReq.Query[i].AllowedPreds = allowedPreds
	}

	return nil
}

func authorizeSchemaQuery(ctx context.Context, er *query.ExecutionResult) error {
	if len(worker.Config.HmacSecret) == 0 {
		// the user has not turned on the acl feature
		return nil
	}

	// find the predicates being sent in response
	preds := make([]string, 0)
	predsMap := make(map[string]struct{})
	for _, predNode := range er.SchemaNode {
		preds = append(preds, predNode.Predicate)
		predsMap[predNode.Predicate] = struct{}{}
	}
	for _, typeNode := range er.Types {
		for _, field := range typeNode.Fields {
			if _, ok := predsMap[field.Predicate]; !ok {
				preds = append(preds, field.Predicate)
			}
		}
	}

	doAuthorizeSchemaQuery := func() (map[string]struct{}, error) {
		userData, err := extractUserAndGroups(ctx)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, err.Error())
		}

		groupIds := userData[1:]
		if x.IsGuardian(groupIds) {
			// Members of guardian groups are allowed to query anything.
			return nil, nil
		}
		result, err := authorizePreds(ctx, userData, preds, acl.Read)
		return result.blocked, err
	}

	// find the predicates which are blocked for the schema query
	blockedPreds, err := doAuthorizeSchemaQuery()
	if err != nil {
		return err
	}

	// remove those predicates from response
	if len(blockedPreds) > 0 {
		respPreds := make([]*pb.SchemaNode, 0)
		for _, predNode := range er.SchemaNode {
			if _, ok := blockedPreds[predNode.Predicate]; !ok {
				respPreds = append(respPreds, predNode)
			}
		}
		er.SchemaNode = respPreds

		for _, typeNode := range er.Types {
			respFields := make([]*pb.SchemaUpdate, 0)
			for _, field := range typeNode.Fields {
				if _, ok := blockedPreds[field.Predicate]; !ok {
					respFields = append(respFields, field)
				}
			}
			typeNode.Fields = respFields
		}
	}

	return nil
}

// AuthGuardianOfTheGalaxy authorizes the operations for the users who belong to the guardians
// group in the galaxy namespace. This authorization is used for admin usages like creation and
// deletion of a namespace, resetting passwords across namespaces etc.
func AuthGuardianOfTheGalaxy(ctx context.Context) error {
	if !x.WorkerConfig.AclEnabled {
		return nil
	}
	ns, err := x.ExtractJWTNamespace(ctx)
	if err != nil {
		return errors.Wrap(err, "Authorize guradian of the galaxy, extracting jwt token, error:")
	}
	if ns != 0 {
		return errors.New("Only guardian of galaxy is allowed to do this operation")
	}
	// AuthorizeGuardians will extract (user, []groups) from the JWT claims and will check if
	// any of the group to which the user belongs is "guardians" or not.
	if err := AuthorizeGuardians(ctx); err != nil {
		return err
	}
	glog.V(2).Info("Successfully authorised the guardian of the galaxy")
	return nil
}

// AuthorizeGuardians authorizes the operation for users which belong to Guardians group.
func AuthorizeGuardians(ctx context.Context) error {
	if len(worker.Config.HmacSecret) == 0 {
		// the user has not turned on the acl feature
		return nil
	}

	userData, err := extractUserAndGroups(ctx)
	switch {
	case err == x.ErrNoJwt:
		return status.Error(codes.PermissionDenied, err.Error())
	case err != nil:
		return status.Error(codes.Unauthenticated, err.Error())
	default:
		userId := userData[0]
		groupIds := userData[1:]

		if !x.IsGuardian(groupIds) {
			// Deny access for members of non-guardian groups
			return status.Error(codes.PermissionDenied, fmt.Sprintf("Only guardians are "+
				"allowed access. User '%v' is not a member of guardians group.", userId))
		}
	}

	return nil
}

/*
	addUserFilterToQuery applies makes sure that a user can access only its own
	acl info by applying filter of userid and groupid to acl predicates. A query like
	Conversion pattern:
		* me(func: type(dgraph.type.Group)) ->
				me(func: type(dgraph.type.Group)) @filter(eq("dgraph.xid", groupIds...))
		* me(func: type(dgraph.type.User)) ->
				me(func: type(dgraph.type.User)) @filter(eq("dgraph.xid", userId))

*/
func addUserFilterToQuery(gq *gql.GraphQuery, userId string, groupIds []string) {
	if gq.Func != nil && gq.Func.Name == "type" {
		// type function only supports one argument
		if len(gq.Func.Args) != 1 {
			return
		}
		arg := gq.Func.Args[0]
		// The case where value of some varialble v (say) is "dgraph.type.Group" and a
		// query comes like `eq(dgraph.type, val(v))`, will be ignored here.
		if arg.Value == "dgraph.type.User" {
			newFilter := userFilter(userId)
			gq.Filter = parentFilter(newFilter, gq.Filter)
		} else if arg.Value == "dgraph.type.Group" {
			newFilter := groupFilter(groupIds)
			gq.Filter = parentFilter(newFilter, gq.Filter)
		}
	}

	gq.Filter = addUserFilterToFilter(gq.Filter, userId, groupIds)

	switch gq.Attr {
	case "dgraph.user.group":
		newFilter := groupFilter(groupIds)
		gq.Filter = parentFilter(newFilter, gq.Filter)
	case "~dgraph.user.group":
		newFilter := userFilter(userId)
		gq.Filter = parentFilter(newFilter, gq.Filter)
	}

	for _, ch := range gq.Children {
		addUserFilterToQuery(ch, userId, groupIds)
	}
}

func parentFilter(newFilter, filter *gql.FilterTree) *gql.FilterTree {
	if filter == nil {
		return newFilter
	}
	parentFilter := &gql.FilterTree{
		Op:    "AND",
		Child: []*gql.FilterTree{filter, newFilter},
	}
	return parentFilter
}

func userFilter(userId string) *gql.FilterTree {
	// A logged in user should always have a userId.
	return &gql.FilterTree{
		Func: &gql.Function{
			Attr: "dgraph.xid",
			Name: "eq",
			Args: []gql.Arg{{Value: userId}},
		},
	}
}

func groupFilter(groupIds []string) *gql.FilterTree {
	// The user doesn't have any groups, so add an empty filter @filter(uid([])) so that all
	// groups are filtered out.
	if len(groupIds) == 0 {
		filter := &gql.FilterTree{
			Func: &gql.Function{
				Name: "uid",
				UID:  []uint64{},
			},
		}
		return filter
	}

	filter := &gql.FilterTree{
		Func: &gql.Function{
			Attr: "dgraph.xid",
			Name: "eq",
		},
	}

	for _, gid := range groupIds {
		filter.Func.Args = append(filter.Func.Args,
			gql.Arg{Value: gid})
	}

	return filter
}

/*
 addUserFilterToFilter makes sure that user can't misue filters to access other user's info.
 If the *filter* have type(dgraph.type.Group) or type(dgraph.type.User) functions,
 it generate a *newFilter* with function like eq(dgraph.xid, userId) or eq(dgraph.xid,groupId...)
 and return a filter of the form

		&gql.FilterTree{
			Op: "AND",
			Child: []gql.FilterTree{
				{filter, newFilter}
			}
		}
*/
func addUserFilterToFilter(filter *gql.FilterTree, userId string,
	groupIds []string) *gql.FilterTree {

	if filter == nil {
		return nil
	}

	if filter.Func != nil && filter.Func.Name == "type" {

		// type function supports only one argument
		if len(filter.Func.Args) != 1 {
			return nil
		}
		arg := filter.Func.Args[0]
		var newFilter *gql.FilterTree
		switch arg.Value {
		case "dgraph.type.User":
			newFilter = userFilter(userId)
		case "dgraph.type.Group":
			newFilter = groupFilter(groupIds)
		}

		// If filter have function, it can't have children.
		return parentFilter(newFilter, filter)
	}

	for idx, child := range filter.Child {
		filter.Child[idx] = addUserFilterToFilter(child, userId, groupIds)
	}

	return filter
}

// removePredsFromQuery removes all the predicates in blockedPreds
// from all the queries in gqs.
func removePredsFromQuery(gqs []*gql.GraphQuery,
	blockedPreds map[string]struct{}) []*gql.GraphQuery {

	filteredGQs := gqs[:0]
L:
	for _, gq := range gqs {
		if gq.Func != nil && len(gq.Func.Attr) > 0 {
			if _, ok := blockedPreds[gq.Func.Attr]; ok {
				continue
			}
		}
		if len(gq.Attr) > 0 {
			if _, ok := blockedPreds[gq.Attr]; ok {
				continue
			}
			if gq.Attr == "val" {
				// TODO (Anurag): If val supports multiple variables, this would
				// need an upgrade
				for _, variable := range gq.NeedsVar {
					if _, ok := blockedPreds[variable.Name]; ok {
						continue L
					}
				}
			}
		}

		order := gq.Order[:0]
		for _, ord := range gq.Order {
			if _, ok := blockedPreds[ord.Attr]; ok {
				continue
			}
			order = append(order, ord)
		}

		gq.Order = order
		gq.Filter = removeFilters(gq.Filter, blockedPreds)
		gq.GroupbyAttrs = removeGroupBy(gq.GroupbyAttrs, blockedPreds)
		gq.Children = removePredsFromQuery(gq.Children, blockedPreds)
		filteredGQs = append(filteredGQs, gq)
	}

	return filteredGQs
}

func removeVarsFromQueryVars(gqs []*gql.Vars,
	blockedVars map[string]struct{}) []*gql.Vars {

	filteredGQs := gqs[:0]
	for _, gq := range gqs {
		var defines []string
		var needs []string
		for _, variable := range gq.Defines {
			if _, ok := blockedVars[variable]; !ok {
				defines = append(defines, variable)
			}
		}
		for _, variable := range gq.Needs {
			if _, ok := blockedVars[variable]; !ok {
				needs = append(needs, variable)
			}
		}
		gq.Defines = defines
		gq.Needs = needs
		filteredGQs = append(filteredGQs, gq)
	}
	return filteredGQs
}

func removeFilters(f *gql.FilterTree, blockedPreds map[string]struct{}) *gql.FilterTree {
	if f == nil {
		return nil
	}
	if f.Func != nil && len(f.Func.Attr) > 0 {
		if _, ok := blockedPreds[f.Func.Attr]; ok {
			return nil
		}
	}

	filteredChildren := f.Child[:0]
	for _, ch := range f.Child {
		child := removeFilters(ch, blockedPreds)
		if child != nil {
			filteredChildren = append(filteredChildren, child)
		}
	}
	if len(filteredChildren) != len(f.Child) && (f.Op == "AND" || f.Op == "NOT") {
		return nil
	}
	f.Child = filteredChildren
	return f
}

func removeGroupBy(gbAttrs []gql.GroupByAttr,
	blockedPreds map[string]struct{}) []gql.GroupByAttr {

	filteredGbAttrs := gbAttrs[:0]
	for _, gbAttr := range gbAttrs {
		if _, ok := blockedPreds[gbAttr.Attr]; ok {
			continue
		}
		filteredGbAttrs = append(filteredGbAttrs, gbAttr)
	}
	return filteredGbAttrs
}
