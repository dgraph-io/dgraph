/*
 * Copyright 2023 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package dgraphtest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/pkg/errors"

	"github.com/dgraph-io/dgo/v230"
	"github.com/dgraph-io/dgo/v230/protos/api"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/x"
)

type Cluster interface {
	Client() (*GrpcClient, func(), error)
	HTTPClient() (*HTTPClient, error)
	AlphasHealth() ([]string, error)
	AlphasLogs() ([]string, error)
	AssignUids(gc *dgo.Dgraph, num uint64) error
	GetVersion() string
}

type GrpcClient struct {
	*dgo.Dgraph
}

// HttpToken stores credentials for making HTTP request
type HttpToken struct {
	UserId       string
	Password     string
	AccessJwt    string
	RefreshToken string
}

// HTTPClient allows doing operations on Dgraph over http
type HTTPClient struct {
	*HttpToken
	adminURL   string
	graphqlURL string
}

// GraphQLParams are used for making graphql requests to dgraph
type GraphQLParams struct {
	Query      string                    `json:"query"`
	Variables  map[string]interface{}    `json:"variables"`
	Extensions *schema.RequestExtensions `json:"extensions,omitempty"`
}

type GraphQLResponse struct {
	Data       json.RawMessage        `json:"data,omitempty"`
	Errors     x.GqlErrorList         `json:"errors,omitempty"`
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

func (hc *HTTPClient) Login(user, password string, ns uint64) error {
	login := `mutation login($userId: String, $password: String, $namespace: Int, $refreshToken: String) {
		login(userId: $userId, password: $password, namespace: $namespace, refreshToken: $refreshToken) {
			response {
				accessJWT
				refreshJWT
			}
		}
	}`
	params := GraphQLParams{
		Query: login,
		Variables: map[string]interface{}{
			"userId":       user,
			"password":     password,
			"namespace":    ns,
			"refreshToken": hc.RefreshToken,
		},
	}

	hc.HttpToken = &HttpToken{
		UserId:   user,
		Password: password,
	}
	return hc.doLogin(params, true)
}

func (hc *HTTPClient) LoginUsingToken(ns uint64) error {
	q := `mutation login( $namespace: Int, $refreshToken:String) {
 		login(namespace: $namespace, refreshToken: $refreshToken) {
 			response {
 				accessJWT
 				refreshJWT
 			}
 		}
 	}`
	params := GraphQLParams{
		Query: q,
		Variables: map[string]interface{}{
			"namespace":    ns,
			"refreshToken": hc.RefreshToken,
		},
	}

	return hc.doLogin(params, true)
}

func (hc *HTTPClient) LoginIntoNamespace(user, password string, ns uint64) error {
	q := `mutation login($userId: String, $password: String, $namespace: Int) {
		login(userId: $userId, password: $password, namespace: $namespace) {
			response {
				accessJWT
				refreshJWT
			}
		}
	}`
	params := GraphQLParams{
		Query: q,
		Variables: map[string]interface{}{
			"userId":    user,
			"password":  password,
			"namespace": ns,
		},
	}

	hc.HttpToken = &HttpToken{
		UserId:   user,
		Password: password,
	}
	return hc.doLogin(params, true)
}

func (hc *HTTPClient) doLogin(params GraphQLParams, isAdmin bool) error {
	resp, err := hc.RunGraphqlQuery(params, isAdmin)
	if err != nil {
		return err
	}
	var r struct {
		Login struct {
			Response struct {
				AccessJWT  string
				RefreshJwt string
			}
		}
	}
	if err := json.Unmarshal(resp, &r); err != nil {
		return errors.Wrap(err, "error unmarshalling response into object")
	}
	if r.Login.Response.AccessJWT == "" {
		return errors.Errorf("no access JWT found in the response")
	}

	hc.HttpToken.AccessJwt = r.Login.Response.AccessJWT
	hc.HttpToken.RefreshToken = r.Login.Response.RefreshJwt
	return nil
}

func (hc *HTTPClient) ResetPassword(userID, newPass string, nsID uint64) (string, error) {
	const query = `mutation resetPassword($userID: String!, $newpass: String!, $namespaceId: Int!){
		resetPassword(input: {userId: $userID, password: $newpass, namespace: $namespaceId}) {
			userId
			message
		}
	}`
	params := GraphQLParams{
		Query: query,
		Variables: map[string]interface{}{
			"namespaceId": nsID,
			"userID":      userID,
			"newpass":     newPass,
		},
	}
	resp, err := hc.RunGraphqlQuery(params, true)
	if err != nil {
		return "", err
	}

	var result struct {
		ResetPassword struct {
			UserId  string `json:"userId"`
			Message string `json:"message"`
		}
	}
	if err = json.Unmarshal(resp, &result); err != nil {
		return "", errors.Wrap(err, "error unmarshalling response")
	}
	if userID != result.ResetPassword.UserId {
		return "", fmt.Errorf("unexpected error while resetting password for user [%s]", userID)
	}
	if result.ResetPassword.Message == "Reset password is successful" {
		return result.ResetPassword.UserId, nil
	}
	return "", errors.New(result.ResetPassword.Message)
}

// doPost makes a post request to the graphql admin endpoint
func (hc *HTTPClient) doPost(body []byte, admin bool) ([]byte, error) {
	url := hc.graphqlURL
	if admin {
		url = hc.adminURL
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, errors.Wrapf(err, "error building req for endpoint [%v]", url)
	}
	req.Header.Add("Content-Type", "application/json")

	if hc.HttpToken != nil {
		req.Header.Add("X-Dgraph-AccessToken", hc.AccessJwt)
	}

	return doReq(req)
}

// RunGraphqlQuery makes a query to graphql (or admin) endpoint
func (hc *HTTPClient) RunGraphqlQuery(params GraphQLParams, admin bool) ([]byte, error) {
	reqBody, err := json.Marshal(params)
	if err != nil {
		return nil, errors.Wrap(err, "error while marshalling params")
	}

	respBody, err := hc.doPost(reqBody, admin)
	if err != nil {
		return nil, errors.Wrap(err, "error while running admin query")
	}

	var gqlResp GraphQLResponse
	if err := json.Unmarshal(respBody, &gqlResp); err != nil {
		return nil, errors.Wrap(err, "error unmarshalling GQL response")
	}
	if len(gqlResp.Errors) > 0 {
		return nil, errors.Wrapf(gqlResp.Errors, "error while running admin query")
	}
	return gqlResp.Data, nil
}

// Backup creates a backup of dgraph at a given path
func (hc *HTTPClient) Backup(forceFull bool, backupPath string) error {
	const query = `mutation backup($dest: String!, $ff: Boolean!) {
		backup(input: {destination: $dest, forceFull: $ff}) {
			response {
				code
			}
			taskId
		}
	}`
	params := GraphQLParams{
		Query:     query,
		Variables: map[string]interface{}{"dest": backupPath, "ff": forceFull},
	}
	resp, err := hc.RunGraphqlQuery(params, true)
	if err != nil {
		return err
	}

	var backupResp struct {
		Backup struct {
			Response struct {
				Code string `json:"code,omitempty"`
			} `json:"response,omitempty"`
			TaskID string `json:"taskId,omitempty"`
		} `json:"backup,omitempty"`
	}
	if err := json.Unmarshal(resp, &backupResp); err != nil {
		return errors.Wrap(err, "error unmarshalling backup response")
	}
	if backupResp.Backup.Response.Code != "Success" {
		return fmt.Errorf("backup failed")
	}
	return hc.WaitForTask(backupResp.Backup.TaskID)
}

// WaitForTask waits for the task to finish
func (hc *HTTPClient) WaitForTask(taskId string) error {
	const query = `query task($id: String!) {
		task(input: {id: $id}) {
			status
		}
	}`
	params := GraphQLParams{
		Query:     query,
		Variables: map[string]interface{}{"id": taskId},
	}

	for {
		time.Sleep(waitDurBeforeRetry)

		resp, err := hc.RunGraphqlQuery(params, true)
		if err != nil {
			return err
		}

		var statusResp struct {
			Task struct {
				Status string `json:"status,omitempty"`
			} `json:"task,omitempty"`
		}
		if err := json.Unmarshal(resp, &statusResp); err != nil {
			return errors.Wrap(err, "error unmarshalling status response")
		}
		switch statusResp.Task.Status {
		case "Success":
			return nil
		case "Failed", "Unknown":
			return fmt.Errorf("task failed with status: %s", statusResp.Task.Status)
		}
	}
}

// Restore performs restore on Dgraph cluster from the given path to backup
func (hc *HTTPClient) Restore(c Cluster, backupPath string,
	backupId string, incrFrom, backupNum int, encKey string) error {

	// incremental restore was introduced in commit 8b3712e93ed2435bea52d957f7b69976c6cfc55b
	beforeIncrRestore, err := isParent("8b3712e93ed2435bea52d957f7b69976c6cfc55b", c.GetVersion())
	if err != nil {
		return errors.Wrapf(err, "error checking incremental restore support")
	}
	if beforeIncrRestore && incrFrom != 0 {
		return errors.New("incremental restore is not supported by the cluster")
	}

	var varPart, queryPart string
	if !beforeIncrRestore {
		varPart = "$incrFrom: Int, "
		queryPart = " incrementalFrom: $incrFrom,"
	}
	query := fmt.Sprintf(`mutation restore($location: String!, $backupId: String,
			%v$backupNum: Int, $encKey: String) {
		restore(input: {location: $location, backupId: $backupId,%v
				backupNum: $backupNum, encryptionKeyFile: $encKey}) {
			code
			message
		}
	}`, varPart, queryPart)
	vars := map[string]interface{}{"location": backupPath, "backupId": backupId,
		"backupNum": backupNum, "encKey": encKey}
	if !beforeIncrRestore {
		vars["incrFrom"] = incrFrom
	}

	params := GraphQLParams{
		Query:     query,
		Variables: vars,
	}
	resp, err := hc.RunGraphqlQuery(params, true)
	if err != nil {
		return err
	}

	var restoreResp struct {
		Restore struct {
			Code    string
			Message string
		}
	}
	if err := json.Unmarshal(resp, &restoreResp); err != nil {
		return errors.Wrap(err, "error unmarshalling restore response")
	}
	if restoreResp.Restore.Code != "Success" {
		return fmt.Errorf("restore failed, response: %+v", restoreResp.Restore)
	}
	return nil
}

// WaitForRestore waits for restore to complete on all alphas
func WaitForRestore(c Cluster) error {
loop:
	for {
		time.Sleep(waitDurBeforeRetry)

		resp, err := c.AlphasHealth()
		if err != nil && strings.Contains(err.Error(), "the server is in draining mode") {
			continue loop
		} else if err != nil {
			return err
		}
		for _, hr := range resp {
			if strings.Contains(hr, "opRestore") {
				continue loop
			}
		}
		break
	}

	// sometimes it is possible that one alpha hasn't even started the restore process.
	// In this case, the alpha will not show "opRestore" in the response to health endpoint
	// and the "loop" will not be able to detect this case. The "loop2" below ensures
	// by looking into the logs that the restore process has in fact been started and completed.
loop2:
	for i := 0; i < 60; i++ {
		time.Sleep(waitDurBeforeRetry)

		alphasLogs, err := c.AlphasLogs()
		if err != nil {
			return err
		}

		for _, alphaLogs := range alphasLogs {
			if !strings.Contains(alphaLogs, "Operation completed with id: opRestore") {
				continue loop2
			}
		}
		return nil
	}

	return errors.Errorf("restore wasn't started on at least 1 alpha")
}

func (hc *HTTPClient) Export(dest string) error {
	const exportRequest = `mutation export($dest: String!, $f: String!) {
		export(input: {destination: $dest, format: $f}) {
			response {
				message
			}
		}
	}`
	params := GraphQLParams{
		Query: exportRequest,
		Variables: map[string]interface{}{
			"dest": dest,
			"f":    "rdf",
		},
	}

	resp, err := hc.RunGraphqlQuery(params, true)
	if err != nil {
		return err
	}
	var r struct {
		Export struct {
			Response struct {
				Message string
			}
		}
	}
	if err := json.Unmarshal(resp, &r); err != nil {
		return errors.Wrap(err, "error unmarshalling export response")
	}
	return nil
}

// UpdateGQLSchema updates graphql schema for a given HTTP client
func (hc *HTTPClient) UpdateGQLSchema(sch string) error {
	const query = `mutation updateGQLSchema($sch: String!) {
		updateGQLSchema(input: { set: { schema: $sch }}) {
			gqlSchema {
				id
			}
		}
	}`
	params := GraphQLParams{
		Query: query,
		Variables: map[string]interface{}{
			"sch": sch,
		},
	}
	if _, err := hc.RunGraphqlQuery(params, true); err != nil {
		return err
	}
	return nil
}

// PostPersistentQuery stores a persisted query
func (hc *HTTPClient) PostPersistentQuery(query, sha string) ([]byte, error) {
	params := GraphQLParams{
		Query: query,
		Extensions: &schema.RequestExtensions{PersistedQuery: schema.PersistedQuery{
			Sha256Hash: sha,
		}},
	}
	return hc.RunGraphqlQuery(params, false)
}

// SetupSchema sets up DQL schema
func (gc *GrpcClient) SetupSchema(dbSchema string) error {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	return gc.Alter(ctx, &api.Operation{Schema: dbSchema})
}

// DropAll drops all the data in the db
func (gc *GrpcClient) DropAll() error {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	return gc.Alter(ctx, &api.Operation{DropAll: true})
}

// DropPredicate drops the predicate from the data in the db
func (gc *GrpcClient) DropPredicate(pred string) error {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	return gc.Alter(ctx, &api.Operation{DropAttr: pred})
}

// Mutate performs a given mutation in a txn
func (gc *GrpcClient) Mutate(rdfs string) (*api.Response, error) {
	txn := gc.NewTxn()
	defer func() { _ = txn.Discard(context.Background()) }()

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	mu := &api.Mutation{SetNquads: []byte(rdfs), CommitNow: true}
	return txn.Mutate(ctx, mu)
}

// Query performa a given query in a new txn
func (gc *GrpcClient) Query(query string) (*api.Response, error) {
	txn := gc.NewTxn()
	defer func() { _ = txn.Discard(context.Background()) }()

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	return txn.Query(ctx, query)
}

// ShouldSkipTest skips a given test if minVersion > clusterVersion
func ShouldSkipTest(t *testing.T, minVersion, clusterVersion string) error {
	isParentCommit, err := isParent(minVersion, clusterVersion)
	if err != nil {
		t.Fatal(err)
	}
	if isParentCommit {
		t.Skipf("test is valid for commits greater than [%v]", minVersion)
	}

	return nil
}

// isParent checks whether ancestor is the ancestor commit of the given descendant commit
func isParent(ancestor, descendant string) (bool, error) {
	// the order of if conditions matters here
	if descendant == localVersion {
		return false, nil
	} else if ancestor == localVersion {
		return false, nil
	}

	repo, err := openDgraphRepo()
	if err != nil {
		return false, err
	}

	ancestorHash, err := repo.ResolveRevision(plumbing.Revision(ancestor))
	if err != nil {
		return false, errors.Wrapf(err, "error while getting reference of [%v]", ancestor)
	}
	ancestorCommit, err := repo.CommitObject(*ancestorHash)
	if err != nil {
		return false, errors.Wrapf(err, "error finding commit object [%v]", ancestor)
	}

	descendantHash, err := repo.ResolveRevision(plumbing.Revision(descendant))
	if err != nil {
		return false, err
	}
	descendantCommit, err := repo.CommitObject(*descendantHash)
	if err != nil {
		return false, errors.Wrapf(err, "error finding commit object [%v]", descendant)
	}

	isParentCommit, err := descendantCommit.IsAncestor(ancestorCommit)
	if err != nil {
		return false, errors.Wrapf(err, "unable to compare commit [%v] to commit [%v]",
			descendant, ancestor)
	}
	return isParentCommit, nil
}
