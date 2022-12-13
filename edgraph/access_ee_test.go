package edgraph

import (
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/ee/acl"
	"github.com/dgraph-io/dgraph/worker"

	jwt "github.com/dgrijalva/jwt-go"
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

		ud, _ := validateToken(tokenString)
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
	expiry := time.Now().Add(worker.Config.AccessJwtTtl).Unix()
	userDataList := []userData{
		{1234567890, "user1", []string{"701", "702"}},
		{2345678901, "user2", []string{"703", "701"}},
		{3456789012, "user3", []string{"702", "703"}},
	}

	for _, userdata := range userDataList {
		tokstr := generateJWT(userdata.namespace, userdata.userId, g, expiry)
		ud1, _ := validateToken (tokstr)
		jwtstr, _ := getAccessJwt(userdata.userId, grpLst, userdata.namespace)
		ud2, _ := validateToken (jwtstr)

		if ud1.namespace != ud2.namespace || ud1.userId != ud2.userId || !sliceCompare(ud1.groupIds, ud2.groupIds) {
			t.Errorf("Generated jwt output %+v is not equal to the access jwt output %+v", ud1, ud2)
		}
	}
}

func TestGetRefreshJwt(t *testing.T) {
	expiry := time.Now().Add(worker.Config.RefreshJwtTtl).Unix()
	userDataList := []userData{
		{1234567890, "user1", []string{"701", "702"}},
		{2345678901, "user2", []string{"703", "701"}},
		{3456789012, "user3", []string{"702", "703"}},
	}

	for _, userdata := range userDataList {
		tokstr := generateJWT(userdata.namespace, userdata.userId, nil, expiry)
		ud1, _ := validateToken (tokstr)
		jwtstr, _ := getRefreshJwt(userdata.userId, userdata.namespace)
		ud2, _ := validateToken (jwtstr)

		if ud1.namespace != ud2.namespace || ud1.userId != ud2.userId || !sliceCompare(ud1.groupIds, ud2.groupIds) {
			t.Errorf("Generated jwt output %+v is not equal to the refresh jwt output %+v", ud1, ud2)
		}
	}
}
