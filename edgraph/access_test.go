/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package edgraph

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"github.com/hypermodeinc/dgraph/v25/acl"
	"github.com/hypermodeinc/dgraph/v25/worker"
	"github.com/hypermodeinc/dgraph/v25/x"
)

func generateJWT(namespace uint64, userId string, groupIds []string, expiry int64) string {
	claims := jwt.MapClaims{"namespace": namespace, "userid": userId, "exp": expiry}
	if groupIds != nil {
		claims["groups"] = groupIds
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &claims)
	tokenString, err := token.SignedString(x.MaybeKeyToBytes(worker.Config.AclSecretKey))
	if err != nil {
		panic(err)
	}

	return tokenString
}

func TestValidateToken(t *testing.T) {
	expiry := time.Now().Add(time.Minute * 30).Unix()
	userDataList := []userData{
		{1234567890, "user1", []string{"701", "702"}},
		{2345678901, "user2", []string{"703", "701"}},
		{3456789012, "user3", []string{"702", "703"}},
	}

	for _, userdata := range userDataList {
		tokenString := generateJWT(userdata.namespace, userdata.userId, userdata.groupIds, expiry)
		ud, err := validateToken(tokenString)
		require.NoError(t, err)
		require.Equal(t, userdata.namespace, ud.namespace)
		require.Equal(t, userdata.userId, ud.userId)
		require.Equal(t, userdata.groupIds, ud.groupIds)
	}
}

func TestGetAccessJwt(t *testing.T) {
	grpLst := []acl.Group{
		{
			Uid:     "100",
			GroupID: "1001",
			Users:   []acl.User{},
			Rules:   []acl.Acl{},
		},
		{
			Uid:     "101",
			GroupID: "1011",
			Users:   []acl.User{},
			Rules:   []acl.Acl{},
		},
		{
			Uid:     "102",
			GroupID: "1021",
			Users:   []acl.User{},
			Rules:   []acl.Acl{},
		},
	}

	g := acl.GetGroupIDs(grpLst)
	userDataList := []userData{
		{1234567890, "user1", []string{"701", "702"}},
		{2345678901, "user2", []string{"703", "701"}},
		{3456789012, "user3", []string{"702", "703"}},
	}

	for _, userdata := range userDataList {
		jwtstr, err := getAccessJwt(userdata.userId, grpLst, userdata.namespace)
		require.NoError(t, err)
		ud, err := validateToken(jwtstr)
		require.NoError(t, err)
		require.Equal(t, userdata.namespace, ud.namespace)
		require.Equal(t, userdata.userId, ud.userId)
		require.Equal(t, g, ud.groupIds)
	}
}

func TestGetRefreshJwt(t *testing.T) {
	userDataList := []userData{
		{1234567890, "user1", []string{"701", "702"}},
		{2345678901, "user2", []string{"703", "701"}},
		{3456789012, "user3", []string{"702", "703"}},
	}

	for _, userdata := range userDataList {
		jwtstr, _ := getRefreshJwt(userdata.userId, userdata.namespace)
		ud, err := validateToken(jwtstr)
		require.NoError(t, err)
		require.Equal(t, userdata.namespace, ud.namespace)
		require.Equal(t, userdata.userId, ud.userId)
	}
}

func TestMain(m *testing.M) {
	worker.Config.AclJwtAlg = jwt.SigningMethodHS256
	x.WorkerConfig.AclJwtAlg = jwt.SigningMethodHS256
	x.WorkerConfig.AclPublicKey = x.Sensitive("6ABBAA2014CFF00289D20D20DA296F67")
	worker.Config.AccessJwtTtl = 20 * time.Second
	worker.Config.RefreshJwtTtl = 20 * time.Second
	worker.Config.AclSecretKey = x.Sensitive("6ABBAA2014CFF00289D20D20DA296F67")
	m.Run()
}
