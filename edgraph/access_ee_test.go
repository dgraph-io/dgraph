package edgraph

import (
        "fmt"
        "testing"
        "time"
        "strings"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/ee/acl"
        jwt "github.com/dgrijalva/jwt-go"
)

var (
        secretkey string = "secretkey"

        userDataList = []userData {
                        userData{1234567890, "user1", []string {"701", "702"}},
                        userData{2345678901, "user2", []string {"703", "701"}},
                        userData{3456789012, "user3", []string {"702", "703"}},

        }
)

func generateJWT (namespace uint64, userId string, groupIds []string, exp int64) (string, error) {
	var e1 int64
	var token *jwt.Token

	if exp == 0 {
		e1 = time.Now().Add(time.Minute * 30).Unix()
	} else {
		e1 = exp
	}

	if groupIds != nil {
		token = jwt.NewWithClaims(jwt.SigningMethodHS256, &jwt.MapClaims {
				"namespace": namespace,
				"userid": userId,
				"groups": groupIds,
				"exp":    e1,
			})
	} else {
		token = jwt.NewWithClaims(jwt.SigningMethodHS256, &jwt.MapClaims {
				"namespace": namespace,
				"userid": userId,
				"exp":    e1,
			})
	}

        tokenString, err := token.SignedString([]byte (worker.Config.HmacSecret))
        if err != nil {
                fmt.Errorf("Something Went Wrong: %s", err.Error())
                return "", err
        }
        return tokenString, nil
}

func sliceCompare (x, y []string) bool {
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

func TestValidateToken (t *testing.T) {
        var (
                err error
                tokenString string
        )

        for _, userdata := range userDataList {
                if tokenString, err = generateJWT (userdata.namespace, userdata.userId, userdata.groupIds, 0); err != nil {
                        t.Errorf ("JWT token string generation error: %s", err)
                        return
                }

                ud, err := validateToken (tokenString)
                if err != nil {
                        t.Errorf ("validateToken() error: %s", err)
                        return
                }
                if ud.namespace != userdata.namespace || ud.userId != userdata.userId || !sliceCompare (ud.groupIds, userdata.groupIds) {
                        t.Errorf ("Actual output %+v is not equal to the expected output %+v", userdata, ud)
                }
        }
}

func TestGetAccessJwt (t *testing.T) {
	var (
		err error
		tokstr string
		jwtstr string
		exp int64 = time.Now().Add(worker.Config.AccessJwtTtl).Unix()
	)

	grpLst := []acl.Group {
					acl.Group {
						"100",
						"1001",
						[]acl.User{},
						[]acl.Acl{},
					},
					acl.Group {
						"101",
						"1011",
						[]acl.User{},
						[]acl.Acl{},
					},
					acl.Group {
						"102",
						"1021",
						[]acl.User{},
						[]acl.Acl{},
					},
	}

	for _, userdata := range userDataList {
		g := acl.GetGroupIDs (grpLst)
		if tokstr, err = generateJWT (userdata.namespace, userdata.userId, g, exp); err != nil {
			t.Errorf ("JWT token string generation error: %s", err)
			return
		}
		t1 := strings.Split (tokstr, ".")

		jwtstr, err = getAccessJwt (userdata.userId, grpLst, userdata.namespace)
		if err != nil {
			t.Errorf ("getAccessJwt () error: %s", err)
			return
		}
		t2 := strings.Split (jwtstr, ".")

		/*
		 * JWT token format = [header].[payload].[signature].
		 * One of the factor [signature] would differ is based on the expiry time.
		 * This will differ for tokstr and jwtstr and hence ignoring it.
		 */
		if t1[0] != t2[0] || t1[1] != t2[1] {
			t.Errorf ("Actual output is not equal to the expected output")
		}
	}
}

func TestGetRefreshJwt (t *testing.T) {
	var (
		err error
		tokstr string
		jwtstr string
		exp int64 = time.Now().Add(worker.Config.RefreshJwtTtl).Unix()
	)

	for _, userdata := range userDataList {
		if tokstr, err = generateJWT (userdata.namespace, userdata.userId, nil, exp); err != nil {
                        t.Errorf ("JWT token string generation error: %s", err)
                        return
                }
		t1 := strings.Split (tokstr, ".")

                jwtstr, err = getRefreshJwt (userdata.userId, userdata.namespace)
		if err != nil {
			t.Errorf ("getAccessJwt () error: %s", err)
                        return
                }
		t2 := strings.Split (jwtstr, ".")

		/*
		 * JWT token format = [header].[payload].[signature].
		 * One of the factor [signature] would differ is based on the expiry time.
		 * This will differ for tokstr and jwtstr and hence ignoring it.
		 */
		if t1[0] != t2[0] || t1[1] != t2[1] {
                        t.Errorf ("Actual output is not equal to the expected output")
                }
        }
}
