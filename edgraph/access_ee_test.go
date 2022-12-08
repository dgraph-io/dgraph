package edgraph

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/ee/acl"
	"github.com/dgraph-io/dgraph/worker"
	jwt "github.com/dgrijalva/jwt-go"
)

var (
	userDataList = []userData{
		{1234567890, "user1", []string{"701", "702"}},
		{2345678901, "user2", []string{"703", "701"}},
		{3456789012, "user3", []string{"702", "703"}},
	}
)

func generateJWT(namespace uint64, userId string, groupIds []string, exp int64) (string, error) {
	var token *jwt.Token

	e1 := exp
	if e1 == 0 {
		e1 = time.Now().Add(time.Minute * 30).Unix()
	}

	claims := jwt.MapClaims{
		"namespace": namespace,
		"userid":    userId,
		"exp":       e1,
	}
	if groupIds != nil {
			claims["groups"] = groupIds
	}
	token = jwt.NewWithClaims(jwt.SigningMethodHS256, &claims)

	tokenString, err := token.SignedString([]byte(worker.Config.HmacSecret))
	if err != nil {
		_ = fmt.Errorf("Something Went Wrong: %s", err.Error())
		return "", err
	}
	return tokenString, nil
}

func sliceCompare(x, y []string) bool {
	if len(x) != len(y) {
		return false
	}

	for i, s := range x {
		if s != y[i] {
			return false
		}
	}

	return true
}

func compareTokenParts(tok1 string, tok2 string, part ...int) bool {
	var (
		match bool = true
	)

	t1 := strings.Split(tok1, ".")
	t2 := strings.Split(tok2, ".")

	for _, p := range part {
		if p <= 0 {
			continue
		}

		if t1[p-1] != t2[p-1] {
			match = false
			break
		}
	}

	return match
}

func TestValidateToken(t *testing.T) {
	var (
		err         error
		tokenString string
	)

	for _, userdata := range userDataList {
		if tokenString, err = generateJWT(userdata.namespace, userdata.userId, userdata.groupIds, 0); err != nil {
			t.Errorf("JWT token string generation error: %s", err)
		}

		ud, err := validateToken(tokenString)
		if err != nil {
			t.Errorf("validateToken() error: %s", err)
		}
		if ud.namespace != userdata.namespace || ud.userId != userdata.userId || !sliceCompare(ud.groupIds, userdata.groupIds) {
			t.Errorf("Actual output %+v is not equal to the expected output %+v", userdata, ud)
		}
	}
}

func TestGetAccessJwt(t *testing.T) {
	var (
		tokstr string
		jwtstr string
		exp    int64 = time.Now().Add(worker.Config.AccessJwtTtl).Unix()
	)

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

	for _, userdata := range userDataList {
		g := acl.GetGroupIDs(grpLst)
		tokstr, _ = generateJWT(userdata.namespace, userdata.userId, g, exp)
		jwtstr, _ = getAccessJwt(userdata.userId, grpLst, userdata.namespace)

		/*
		 * JWT token format = [header].[payload].[signature].
		 * One of the factor [signature] would differ is based on the expiry time.
		 * This will differ for tokstr and jwtstr and hence ignoring it.
		 */
		match := compareTokenParts(tokstr, jwtstr, 1, 2, 0) // compare 1st and 2nd part, ignore the 3rd.
		if match == false {
			t.Errorf("Actual output is not equal to the expected output")
		}
	}
}

func TestGetRefreshJwt(t *testing.T) {
	var (
		tokstr string
		jwtstr string
		exp    int64 = time.Now().Add(worker.Config.RefreshJwtTtl).Unix()
	)

	for _, userdata := range userDataList {
		tokstr, _ = generateJWT(userdata.namespace, userdata.userId, nil, exp)
		jwtstr, _ = getRefreshJwt(userdata.userId, userdata.namespace)

		/*
		 * JWT token format = [header].[payload].[signature]
		 * One of the factor [signature] would differ is based on the expiry time.
		 * This will differ for tokstr and jwtstr and hence ignoring it.
		 */
		match := compareTokenParts(tokstr, jwtstr, 1, 2, 0) // compare 1st and 2nd part, ignore the 3rd.
		if match == false {
			t.Errorf("Actual output is not equal to the expected output")
		}
	}
}
