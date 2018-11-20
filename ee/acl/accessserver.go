package acl

import (
	"context"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/edgraph"
	"github.com/dgraph-io/dgraph/ee/acl/cmd"
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
	resp := &api.LogInResponse{
		Context: &api.TxnContext{},
	}

	dbUser, err := queryDBUser(ctx, request.Userid)
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

func queryDBUser(ctx context.Context, userid string) (dbUser *acl.DBUser, err error) {
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
	dbUser, err = acl.UnmarshallDBUser(queryResp, "user")
	if err != nil {
		return nil, err
	}
	return dbUser, nil
}

func toJwtGroups(groups []acl.DBGroup) []JwtGroup {
	jwtGroups := make([]JwtGroup, len(groups))

	for _, g := range groups {
		jwtGroups = append(jwtGroups, JwtGroup{
			Group: g.GroupID,
			Wildcardacl: "", // TODO set it to the wild card acl returned from DB
		})
	}
	return jwtGroups
}




