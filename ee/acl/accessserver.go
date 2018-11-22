package acl

import (
	"context"
	"fmt"
	"time"

	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/edgraph"
	"github.com/dgraph-io/dgraph/ee/acl/cmd"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
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
	if err := validateLoginRequest(request); err != nil {
		return nil, err
	}

	resp := &api.LogInResponse{
		Code: api.AclResponseCode_UNAUTHENTICATED,
	}

	user, err := queryUser(ctx, request.Userid)
	if err != nil {
		glog.Warningf("Unable to login user with user id: %v", request.Userid)
		return nil, err
	}
	if user == nil {
		errMsg := fmt.Sprintf("User not found for user id %v", request.Userid)
		glog.Errorf(errMsg)
		return nil, fmt.Errorf(errMsg)
	}

	if len(user.Password) == 0 {
		errMsg := fmt.Sprintf("Unable to authenticate since the user's password is empty: %v", request.Userid)
		glog.Warning(errMsg)
		return nil, fmt.Errorf(errMsg)
	}

	if user.Password != request.Password {
		glog.Warningf("Password mismatch for user: %v", request.Userid)
		resp.Code = api.AclResponseCode_UNAUTHENTICATED
		return resp, nil
	}

	jwt := &Jwt{
		Header: StdJwtHeader,
		Payload: JwtPayload{
			Userid: request.Userid,
			Groups: toJwtGroups(user.Groups),
			// TODO add the token refresh mechanism and reduce the expiration interval
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

func validateLoginRequest(request *api.LogInRequest) error {
	if request == nil {
		return fmt.Errorf("the request should not be nil")
	}
	if len(request.Userid) == 0 {
		return fmt.Errorf("the userid should not be empty")
	}
	if len(request.Password) == 0 {
		return fmt.Errorf("the password should not be empty")
	}
	return nil
}

func queryUser(ctx context.Context, userid string) (user *acl.User, err error) {
	queryUid := fmt.Sprintf(`
    query search($userid: string){
      user(func: eq(%v, $userid)) {
	    uid,
        %v
        %v {
          uid
          dgraph.xid
        }
      }
    }`, x.Acl_XId, x.Acl_Password, x.Acl_UserGroup)

	queryVars := make(map[string]string)
	queryVars["$userid"] = userid
	queryRequest := api.Request{
		Query: queryUid,
		Vars:  queryVars,
	}

	queryResp, err := (&edgraph.Server{}).Query(ctx, &queryRequest)
	if err != nil {
		glog.Errorf("Error while query user with id %s: %v", userid, err)
		return nil, err
	}
	user, err = acl.UnmarshallUser(queryResp, "user")
	if err != nil {
		return nil, err
	}
	return user, nil
}

func toJwtGroups(groups []acl.Group) []JwtGroup {
	jwtGroups := make([]JwtGroup, len(groups))

	for _, g := range groups {
		jwtGroups = append(jwtGroups, JwtGroup{
			Group:       g.GroupID,
			Wildcardacl: "", // TODO set it to the wild card acl returned from DB
		})
	}
	return jwtGroups
}
