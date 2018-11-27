package acl

import (
	"context"
	"fmt"
	"time"

	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/edgraph"
	"github.com/dgraph-io/dgraph/ee/acl/cmd"
	"github.com/golang/glog"
	"go.opencensus.io/trace"
	otrace "go.opencensus.io/trace"
	"google.golang.org/grpc/peer"
)

// Server implements protos.DgraphAccessServer
type AccessServer struct{}

type AccessOptions struct {
	HmacSecret []byte
	JwtTtl     time.Duration
}

var accessConfig AccessOptions

func SetAccessConfiguration(newConfig AccessOptions) {
	accessConfig = newConfig
}

func (accessServer *AccessServer) LogIn(ctx context.Context,
	request *api.LogInRequest) (*api.LogInResponse, error) {
	ctx, span := otrace.StartSpan(ctx, "accessserver.LogIn")
	defer span.End()

	// record the client ip for this login request
	clientPeer, ok := peer.FromContext(ctx)
	if ok {
		span.Annotate([]trace.Attribute{
			trace.StringAttribute("client_ip", clientPeer.Addr.String()),
		}, "client ip for login")
	}

	if err := validateLoginRequest(request); err != nil {
		return nil, err
	}

	resp := &api.LogInResponse{
		Context: &api.TxnContext{}, // context needed in order to set the jwt below
		Code:    api.AclResponseCode_UNAUTHENTICATED,
	}

	user, err := queryUser(ctx, request.Userid, request.Password)
	if err != nil {
		glog.Warningf("Unable to login user with user id: %v", request.Userid)
		return nil, err
	}
	if user == nil {
		errMsg := fmt.Sprintf("User not found for user id %v", request.Userid)
		glog.Warningf(errMsg)
		return nil, fmt.Errorf(errMsg)
	}
	if !user.PasswordMatch {
		errMsg := fmt.Sprintf("Password mismatch for user: %v", request.Userid)
		glog.Warningf(errMsg)
		return nil, fmt.Errorf(errMsg)
	}

	jwt := &Jwt{
		Header: StdJwtHeader,
		Payload: JwtPayload{
			Userid: request.Userid,
			Groups: toJwtGroups(user.Groups),
			// TODO add the token refresh mechanism
			Exp: time.Now().Add(accessConfig.JwtTtl).Unix(), // set the jwt valid for 30 days
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

func queryUser(ctx context.Context, userid string, password string) (user *acl.User, err error) {
	queryUid := `
    query search($userid: string, $password: string){
      user(func: eq(dgraph.xid, $userid)) {
	    uid
        password_match: checkpwd(dgraph.password, $password)
        dgraph.user.group {
          uid
          dgraph.xid
        }
      }
    }`

	queryVars := make(map[string]string)
	queryVars["$userid"] = userid
	queryVars["$password"] = password
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
	if groups == nil {
		// the user does not have any groups
		return nil
	}

	jwtGroups := make([]JwtGroup, len(groups))
	for _, g := range groups {
		jwtGroups = append(jwtGroups, JwtGroup{
			Group: g.GroupID,
		})
	}
	return jwtGroups
}
