package debugoff

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"testing"

	"github.com/dgraph-io/dgraph/graphql/authorization"
	"github.com/dgraph-io/dgraph/graphql/e2e/common"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

var (
	metaInfo *testutil.AuthMeta
)

type TestCase struct {
	user      string
	role      string
	result    string
	name      string
	variables map[string]interface{}
}

func TestAddGQL(t *testing.T) {
	testCases := []TestCase{{
		user:   "user1",
		result: `{"addUserSecret":{"usersecret":[{"aSecret":"secret1"}]}}`,
		variables: map[string]interface{}{"user": &common.UserSecret{
			ASecret: "secret1",
			OwnedBy: "user1",
		}},
	}, {
		user:   "user2",
		result: ``,
		variables: map[string]interface{}{"user": &common.UserSecret{
			ASecret: "secret2",
			OwnedBy: "user1",
		}},
	}}

	query := `
		mutation addUser($user: AddUserSecretInput!) {
			addUserSecret(input: [$user]) {
				userSecret {
					aSecret
				}
			}
		}
	`
	var expected, result struct {
		AddUserSecret struct {
			UserSecret []*common.UserSecret
		}
	}

	for _, tcase := range testCases {
		getUserParams := &common.GraphQLParams{
			Headers:   common.GetJWT(t, tcase.user, tcase.role, metaInfo),
			Query:     query,
			Variables: tcase.variables,
		}
		gqlResponse := getUserParams.ExecuteAsPost(t, common.GraphqlURL)
		if tcase.result == "" {
			require.Equal(t, len(gqlResponse.Errors), 0)
			continue
		}

		require.Nil(t, gqlResponse.Errors)

		err := json.Unmarshal([]byte(tcase.result), &expected)
		require.NoError(t, err)
		err = json.Unmarshal([]byte(gqlResponse.Data), &result)
		require.NoError(t, err)

		opt := cmpopts.IgnoreFields(common.UserSecret{}, "Id")
		if diff := cmp.Diff(expected, result, opt); diff != "" {
			t.Errorf("result mismatch (-want +got):\n%s", diff)
		}

		for _, i := range result.AddUserSecret.UserSecret {
			i.Delete(t, tcase.user, tcase.role, metaInfo)
		}
	}
}

func TestMain(m *testing.M) {
	schemaFile := "../schema.graphql"
	schema, err := ioutil.ReadFile(schemaFile)
	if err != nil {
		panic(err)
	}

	jsonFile := "../test_data.json"
	data, err := ioutil.ReadFile(jsonFile)
	if err != nil {
		panic(errors.Wrapf(err, "Unable to read file %s.", jsonFile))
	}

	jwtAlgo := []string{authorization.HMAC256, authorization.RSA256}
	for _, algo := range jwtAlgo {
		authSchema, err := testutil.AppendAuthInfo(schema, algo, "../sample_public_key.pem")
		if err != nil {
			panic(err)
		}

		authMeta, err := authorization.Parse(string(authSchema))
		if err != nil {
			panic(err)
		}

		metaInfo = &testutil.AuthMeta{
			PublicKey:      authMeta.VerificationKey,
			Namespace:      authMeta.Namespace,
			Algo:           authMeta.Algo,
			Header:         authMeta.Header,
			PrivateKeyPath: "../sample_private_key.pem",
		}

		common.BootstrapServer(authSchema, data)
		// Data is added only in the first iteration, but the schema is added every iteration.
		if data != nil {
			data = nil
		}
		exitCode := m.Run()
		if exitCode != 0 {
			os.Exit(exitCode)
		}
	}
	os.Exit(0)
}
