package main

import (
	"context"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/dgraph/cmd/alpha"
	"github.com/dgraph-io/dgraph/x"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"strconv"
	"testing"
	"time"
)

const (
	userid = "alice"
	userpassword = "simplepassword"
)

func TestAcl(t *testing.T) {
	wrap := func(fn func(*testing.T, api.DgraphAdminClient)) func(*testing.T) {
		return func(t *testing.T) {
			conn, err := grpc.Dial("localhost:9180", grpc.WithInsecure())
			x.Check(err)

			adminStub := api.NewDgraphAdminClient(conn)
			fn(t, adminStub)
		}
	}
	t.Run("create user", wrap(CreateUser))
	t.Run("login", wrap(LogIn))
}

func CreateUser(t *testing.T, adminClient api.DgraphAdminClient) {
	user := api.AclUser{
		Userid: userid,
		Password: userpassword,
	}

	createUserRequest := api.CreateUserRequest{
		User: &user,
	}

	ctx := context.Background()
	response, err := adminClient.CreateUser(ctx, &createUserRequest)
	require.NoError(t, err)

	userId, err := strconv.ParseInt(response.Uid, 0, 64)
	require.NoError(t, err)
	if userId <= 0 {
		t.Errorf("The user uid must be greater than 0, received %d", userId)
	}

	// creating the user again with the same userid will get a response with a conflict code
	response2, err := adminClient.CreateUser(ctx, &createUserRequest)
	require.NoError(t, err)
	if response2.Code != api.AclResponseCode_CONFLICT {
		t.Errorf("Creating the same user again with the same user id should result in the" +
			" CONFLICT code, received %s", response2.Code)
	}

	userId2, err := strconv.ParseInt(response2.Uid, 0, 64)
	require.NoError(t, err)
	if userId != userId2 {
		t.Errorf("Received different user xids when creating the user again with the same "+
			"user id with the first xid = %d and the second xid = %d", userId, userId2)
	}
}

func LogIn(t *testing.T, adminClient api.DgraphAdminClient) {
	// create user first
	CreateUser(t, adminClient)
	ctx := context.Background()
	loginWithWrongPassword(t, ctx, adminClient)
	loginWithCorrectPassword(t, ctx, adminClient)
}

func loginWithCorrectPassword(t *testing.T, ctx context.Context,
	adminClient api.DgraphAdminClient) {
	// login again with the correct password
	loginRequest := api.LogInRequest{
		Userid:   userid,
		Password: userpassword,
	}
	response2, err := adminClient.LogIn(ctx, &loginRequest)
	require.NoError(t, err)
	if response2.Code != api.AclResponseCode_OK {
		t.Errorf("Login with the correct password should result in the code %v",
			api.AclResponseCode_OK)
	}
	jwt := alpha.Jwt{}
	jwt.DecodeString(response2.Context.Jwt, false, nil)
	//t.Logf("Received jwt from the server: %+v", jwt)
	if jwt.Payload.Userid != userid {
		t.Errorf("the jwt token should have the user id encoded")
	}
	jwtTime := time.Unix(jwt.Payload.Exp, 0)
	jwtValidDays := jwtTime.Sub(time.Now()).Round(time.Hour).Hours() / 24
	if jwtValidDays != 30.0 {
		t.Errorf("The jwt token should be valid for 30 days, received %v days", jwtValidDays)
	}
}

func loginWithWrongPassword(t *testing.T, ctx context.Context,
	adminClient api.DgraphAdminClient) {
	// first try to log in with a wrong password
	loginRequestWithWrongPassword := api.LogInRequest{
		Userid:   userid,
		Password: userpassword + "123",
	}

	response, err := adminClient.LogIn(ctx, &loginRequestWithWrongPassword)
	require.NoError(t, err)
	if response.Code != api.AclResponseCode_UNAUTHENTICATED {
		t.Errorf("Login with the wrong password should result in the code %v", api.AclResponseCode_UNAUTHENTICATED)
	}
}
