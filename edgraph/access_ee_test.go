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

package edgraph

import (
	"testing"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/ee/acl"
	"github.com/dgraph-io/dgraph/worker"
)

func generateJWT(namespace uint64, userId string, groupIds []string, expiry int64) string {
	claims := jwt.MapClaims{"namespace": namespace, "userid": userId, "exp": expiry}
	if groupIds != nil {
		claims["groups"] = groupIds
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &claims)

	tokenString, _ := token.SignedString([]byte(worker.Config.HmacSecret))

	return tokenString
}

func sliceCompare(x, y []string) bool {
	if len(x) != len(y) {
		return false
	}

	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}

	return true
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
		require.Nil(t, err)
		if ud.namespace != userdata.namespace || ud.userId != userdata.userId || !sliceCompare(ud.groupIds, userdata.groupIds) {
			t.Errorf("Actual output %+v is not equal to the expected output %+v", userdata, ud)
		}
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
		jwtstr, _ := getAccessJwt(userdata.userId, grpLst, userdata.namespace)
		ud, err := validateToken(jwtstr)
		require.Nil(t, err)
		if ud.namespace != userdata.namespace || ud.userId != userdata.userId || !sliceCompare(ud.groupIds, g) {
			t.Errorf("Actual output {%v %v %v} is not equal to the output %v generated from"+
				" getAccessJwt() token", userdata.namespace, userdata.userId, grpLst, ud)
		}
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
		require.Nil(t, err)
		if ud.namespace != userdata.namespace || ud.userId != userdata.userId {
			t.Errorf("Actual output {%v %v} is not equal to the output {%v %v} generated from"+
				"getRefreshJwt() token", userdata.namespace, userdata.userId, ud.namespace, ud.userId)
		}
	}
}
