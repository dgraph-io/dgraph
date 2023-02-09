//go:build !oss
// +build !oss

/*
 * Copyright 2023 Dgraph Labs, Inc. All rights reserved.
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/main/licenses/DCL.txt
 */

package edgraph

import (
	"testing"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/ee/acl"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
)

func generateJWT(namespace uint64, userId string, groupIds []string, expiry int64, key string) string {
	claims := jwt.MapClaims{"namespace": namespace, "userid": userId, "exp": expiry}
	if groupIds != nil {
		claims["groups"] = groupIds
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &claims)

	tokenString, _ := token.SignedString([]byte(key))

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
func TestPublicKey(t *testing.T) {
	tokenString := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE3NzU3OTg5MDQsImdyb3VwcyI6WyI3MDEiLCI3MDIiXSwibmFtZXNwYWNlIjoxMjM0NTY3ODkwLCJ1c2VyaWQiOiJ1c2VyMSJ9.kVWrSQgWatrKToaS3DDxAqIx2QGx_6ubkuyCyTTLF8AjkaHLN2nDa-HhW5G_aGxh4-l_X6gCsyay2wUs9cn2R6VD53AUrEdEtF0davsTX0Q1ar-RRo8ShTHj0ufgLn7zRGC7grbZrNZJi1RtDWGDRKzzt67nG-a2CooyP40s-1I--g9qQJoWu69moWiDH6ac2tLo7-wBRmgtWn-SgXE-nJYVDOUcaNWSQBwEX5Pe5L2OUYiusCxnlBhpSSREGYcnXdx9jPKEhfl9atg9QkJSwGyRrCtGY_6gA8tvSZdB2rd6pgXamoxxBadEr_eURg0OeJAJ8wf_IlpS6boOhKNt5Q"
	pemString := `-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAyOhWtgb6lOOFQWhDNBqh
y8Y6Jz52hbJWxpndNBviZzRwVccFXQiFVDzYmNlus+lASGxweptIgY7hqSg8WRIC
cXulMgFNU5HfD3s7+3GZeSl305NBWK7DAXlZPUdVBNrLpNpdPF3wVb5qhaRHZp+t
gXtNsUNnm+w21VPi+07yr/IHeitPXVMr8LWDh5eSVTlxolizYJIRrMI7m84XGjeQ
m9GXi74trzJCrQJU/FTgp9tHA1t/uGN4HfM02SHVkG932iBDJiyR+Q4wO2L+RENo
LkxUfgb6zjAspcDU25V/JAKpvcRUvLJympBK/FTHa/oVT7l/V0E5al/DSEUeU2uo
sQIDAQAB
-----END PUBLIC KEY-----`

	x.WorkerConfig = x.WorkerOptions{
		HmacSecret:   []byte(pemString),
		UsePublicKey: true,
	}
	ud, err := validateToken(tokenString)
	require.Nil(t, err)
	if ud.namespace != 1234567890 || ud.userId != "user1" {
		t.Errorf("Actual output %+v is not equal to the expected output", ud)
	}

}
func TestValidateToken(t *testing.T) {
	expiry := time.Now().Add(time.Minute * 30).Unix()
	userDataList := []userData{
		{1234567890, "user1", []string{"701", "702"}},
		{2345678901, "user2", []string{"703", "701"}},
		{3456789012, "user3", []string{"702", "703"}},
	}
	key := "6ABBAA2014CFF00289D20D20DA296F67"
	x.WorkerConfig = x.WorkerOptions{
		HmacSecret: []byte(key),
	}

	for _, userdata := range userDataList {
		tokenString := generateJWT(userdata.namespace, userdata.userId, userdata.groupIds, expiry, key)
		ud, err := validateToken(tokenString)
		require.NoError(t, err)
		if ud.namespace != userdata.namespace || ud.userId != userdata.userId ||
			!sliceCompare(ud.groupIds, userdata.groupIds) {

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

	worker.Config.AccessJwtTtl = 20 * time.Second
	x.WorkerConfig.HmacSecret = []byte("6ABBAA2014CFF00289D20D20DA296F67")
	for _, userdata := range userDataList {
		jwtstr, err := getAccessJwt(userdata.userId, grpLst, userdata.namespace)
		require.NoError(t, err)
		ud, err := validateToken(jwtstr)
		require.NoError(t, err)
		if ud.namespace != userdata.namespace || ud.userId != userdata.userId || !sliceCompare(ud.groupIds, g) {
			t.Errorf("Actual output {%v %v %v} is not equal to the output %v generated from"+
				" getAccessJwt() token", userdata.namespace, userdata.userId, grpLst, ud)
		}
	}
}

func TestGetRefreshJwt(t *testing.T) {
	x.WorkerConfig.HmacSecret = []byte("6ABBAA2014CFF00289D20D20DA296F67")
	userDataList := []userData{
		{1234567890, "user1", []string{"701", "702"}},
		{2345678901, "user2", []string{"703", "701"}},
		{3456789012, "user3", []string{"702", "703"}},
	}

	for _, userdata := range userDataList {
		jwtstr, _ := getRefreshJwt(userdata.userId, userdata.namespace)
		ud, err := validateToken(jwtstr)
		require.NoError(t, err)
		if ud.namespace != userdata.namespace || ud.userId != userdata.userId {
			t.Errorf("Actual output {%v %v} is not equal to the output {%v %v} generated from"+
				"getRefreshJwt() token", userdata.namespace, userdata.userId, ud.namespace, ud.userId)
		}
	}
}
