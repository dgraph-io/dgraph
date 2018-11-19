package acl

import (
	"context"
	"encoding/json"
	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/edgraph"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	"time"
)

// Server implements protos.DgraphAccessServer
type AccessServer struct{}

type AccessOptions struct {
	HmacSecret []byte
}

var accessConfig AccessOptions

func SetAccessConfiguration(newConfig AccessOptions) {
	accessConfig = newConfig
}

func (accessServer *AccessServer) LogIn(ctx context.Context,
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


	resp.Context.Jwt, err = jwt.EncodeToString(accessConfig.HmacSecret)
	if err != nil {
		glog.Errorf("Unable to encode jwt to string: %v", err)
		return nil, err
	}
	resp.Code = api.AclResponseCode_OK
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

