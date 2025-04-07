/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package dgraphapi

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pkg/errors"
)

var (
	errACLGroupCountMoreThanOne = errors.New("acl group count is other than 1")
)

type AclRule struct {
	Predicate  string `json:"predicate"`
	Permission int32  `json:"permission"`
}

type AclGroup struct {
	Name  string    `json:"name"`
	Rules []AclRule `json:"rules"`
}

func (hc *HTTPClient) CreateUser(username, password string) (string, error) {
	const query = `mutation addUser($name: String!, $pass: String!) {
		 addUser(input: [{name: $name, password: $pass}]) {
			 user {
				 name
			 }
		 }
	 }`
	params := GraphQLParams{
		Query: query,
		Variables: map[string]interface{}{
			"name": username,
			"pass": password,
		},
	}
	resp, err := hc.RunGraphqlQuery(params, true)
	if err != nil {
		return "", err
	}

	var r struct {
		AddUser struct {
			User []struct {
				Name string
			}
		}
	}
	if err := json.Unmarshal(resp, &r); err != nil {
		return "", errors.Wrap(err, "error unmarshalling response")
	}
	return r.AddUser.User[0].Name, nil
}

func (hc *HTTPClient) GetCurrentUser() (string, error) {
	const query = `query {
		getCurrentUser {
			name
		}
	}`
	params := GraphQLParams{Query: query}
	resp, err := hc.RunGraphqlQuery(params, true)
	if err != nil {
		return "", errors.Wrapf(err, "received response: %v", string(resp))
	}

	var userResp struct {
		GetCurrentUser struct {
			Name string
		}
	}
	if err := json.Unmarshal(resp, &userResp); err != nil {
		return "", errors.Wrapf(err, "error unmarshalling getCurrentUser response")
	}
	return userResp.GetCurrentUser.Name, nil
}

func (hc *HTTPClient) DeleteUser(username string) error {
	const query = `mutation deleteUser($name: String!) {
		deleteUser(filter: {name: {eq: $name}}) {
			msg
			numUids
		}
	}`
	params := GraphQLParams{
		Query:     query,
		Variables: map[string]interface{}{"name": username},
	}
	_, err := hc.RunGraphqlQuery(params, true)
	return err
}

func (hc *HTTPClient) CreateGroup(name string) (string, error) {
	const query = `mutation addGroup($name: String!) {
		addGroup(input: [{name: $name}]) {
			group {
				name
			}
		}
	}`
	params := GraphQLParams{
		Query:     query,
		Variables: map[string]interface{}{"name": name},
	}
	resp, err := hc.RunGraphqlQuery(params, true)
	if err != nil {
		return "", err
	}
	type Response struct {
		AddGroup struct {
			Group []struct {
				Name string
			}
		}
	}
	var r Response
	if err := json.Unmarshal(resp, &r); err != nil {
		return "", errors.Wrap(err, "error unmarshalling respnose")
	}
	return r.AddGroup.Group[0].Name, nil
}

func (hc *HTTPClient) CreateGroupWithRules(name string, rules []AclRule) (*AclGroup, error) {
	const query = `mutation addGroup($name: String!, $rules: [RuleRef]){
		addGroup(input: [{name: $name rules: $rules	}]) {
			group {
				name
				rules {
					predicate
					permission
				}
			}
		}
	}`
	params := GraphQLParams{
		Query: query,
		Variables: map[string]interface{}{
			"name":  name,
			"rules": rules,
		},
	}
	resp, err := hc.RunGraphqlQuery(params, true)
	if err != nil {
		return nil, err
	}

	var addGroupResp struct {
		AddGroup struct {
			Group []AclGroup
		}
	}
	if err := json.Unmarshal(resp, &addGroupResp); err != nil {
		return nil, errors.Wrap(err, "error while unmarshalling")
	}
	if len(addGroupResp.AddGroup.Group) != 1 {
		return nil, errACLGroupCountMoreThanOne
	}
	return &addGroupResp.AddGroup.Group[0], nil
}

func (hc *HTTPClient) DeleteGroup(name string) error {
	const query = `mutation deleteGroup($name: String!) {
		deleteGroup(filter: {name: {eq: $name}}) {
			msg
			numUids
		}
	}`
	params := GraphQLParams{
		Query: query,
		Variables: map[string]interface{}{
			"name": name,
		},
	}
	_, err := hc.RunGraphqlQuery(params, true)
	return err
}

func (hc *HTTPClient) AddRulesToGroup(group string, rules []AclRule, newGroup bool) error {
	const query = `mutation updateGroup($name: String!, $rules: [RuleRef!]!) {
		updateGroup(input: {filter: {name: {eq: $name}},set: {rules: $rules}}) {
			group {
				name
				rules {
					predicate
					permission
				}
			}
		}
	}`
	params := GraphQLParams{
		Query: query,
		Variables: map[string]interface{}{
			"name":  group,
			"rules": rules,
		},
	}
	resp, err := hc.RunGraphqlQuery(params, true)
	if err != nil {
		return err
	}
	if !newGroup {
		return nil
	}

	rulesb, err := json.Marshal(rules)
	if err != nil {
		return errors.Wrap(err, "error marshalling rules")
	}
	expectedOutput := fmt.Sprintf(`{
			"updateGroup": {
				"group": [{
					"name": "%s",
					"rules": %s
				}]
			}
		}`, group, rulesb)
	if err := CompareJSON(expectedOutput, string(resp)); err != nil {
		return errors.Wrap(err, "unexpected repsonse")
	}
	return nil
}

func (hc *HTTPClient) AddUserToGroup(userName, group string) error {
	const query = `mutation updateUser($name: String!, $group: String!) {
		updateUser(input: {filter: {name: {eq: $name}},set: {groups: [{ name: $group }]}}) {
			user {
				name
				groups {
					name
				}
			}
		}
	}`
	params := GraphQLParams{
		Query: query,
		Variables: map[string]interface{}{
			"name":  userName,
			"group": group,
		},
	}
	resp, err := hc.RunGraphqlQuery(params, true)
	if err != nil {
		return err
	}
	var result struct {
		UpdateUser struct {
			User []struct {
				Name   string
				Groups []struct {
					Name string
				}
			}
			Name string
		}
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return errors.Wrap(err, "error unmarshalling response")
	}

	if len(result.UpdateUser.User) != 1 {
		return errACLGroupCountMoreThanOne
	}
	if userName != result.UpdateUser.User[0].Name {
		return errors.Errorf("username should be same, expected:%v, actual:%v", userName, result.UpdateUser.User[0].Name)
	}

	var foundGroup bool
	for _, usr := range result.UpdateUser.User {
		for _, grp := range usr.Groups {
			if grp.Name == group {
				foundGroup = true
				break
			}
		}
	}
	if !foundGroup {
		return errors.New("group name should be present in response")
	}
	return nil
}

func (hc *HTTPClient) RemoveUserFromGroup(userName, groupName string) error {
	const query = `mutation updateUser($name: String!, $groupName: String!) {
		updateUser(input: {filter: {name: {eq: $name}},remove: {groups: [{ name: $groupName }]}}) {
			user {
				name
				groups {
					name
				}
			}
		}
	}`
	params := GraphQLParams{
		Query: query,
		Variables: map[string]interface{}{
			"name":      userName,
			"groupName": groupName,
		},
	}
	_, err := hc.RunGraphqlQuery(params, true)
	return err
}

func (hc *HTTPClient) RemovePredicateFromGroup(group, predicate string) error {
	const query = `mutation updateGroup($name: String!, $rules: [String!]!) {
		updateGroup(input: {filter: {name: {eq: $name}},remove: {rules: $rules}}) {
			group {
				name
				rules {
					predicate
					permission
				}
			}
		}
	}`
	params := GraphQLParams{
		Query: query,
		Variables: map[string]interface{}{
			"name":  group,
			"rules": []string{predicate},
		},
	}
	_, err := hc.RunGraphqlQuery(params, true)
	return err
}

func (hc *HTTPClient) UpdateGroup(name string, setRules []AclRule, removeRules []string) (*AclGroup, error) {
	const query = `mutation updateGroup($name: String!, $set: SetGroupPatch, $remove: RemoveGroupPatch){
		updateGroup(input: {filter: {name: {eq: $name}}set: $set remove: $remove}) {
			group {
				name
				rules {
					predicate
					permission
				}
			}
		}
	}`
	params := GraphQLParams{
		Query: query,
		Variables: map[string]interface{}{
			"name":   name,
			"set":    nil,
			"remove": nil,
		},
	}
	if len(setRules) != 0 {
		params.Variables["set"] = map[string]interface{}{
			"rules": setRules,
		}
	}
	if len(removeRules) != 0 {
		params.Variables["remove"] = map[string]interface{}{
			"rules": removeRules,
		}
	}
	resp, err := hc.RunGraphqlQuery(params, true)
	if err != nil {
		return nil, err
	}

	var result struct {
		UpdateGroup struct {
			Group []AclGroup
		}
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.Wrap(err, "error unmarshalling response")
	}
	if len(result.UpdateGroup.Group) != 1 {
		return nil, errACLGroupCountMoreThanOne
	}
	return &result.UpdateGroup.Group[0], nil
}

func (hc *HTTPClient) AddNamespace() (uint64, error) {
	const createNs = `mutation {
		addNamespace {
			namespaceId
			message
		}
	}`

	params := GraphQLParams{Query: createNs}
	resp, err := hc.RunGraphqlQuery(params, true)
	if err != nil {
		return 0, err
	}

	var result struct {
		AddNamespace struct {
			NamespaceId uint64 `json:"namespaceId"`
			Message     string `json:"message"`
		}
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return 0, errors.Wrap(err, "error unmarshalling response")
	}
	if strings.Contains(result.AddNamespace.Message, "Created namespace successfully") {
		return result.AddNamespace.NamespaceId, nil
	}
	return 0, errors.New(result.AddNamespace.Message)
}

func (hc *HTTPClient) DeleteNamespace(nsID uint64) (uint64, error) {
	const deleteReq = `mutation deleteNamespace($namespaceId: Int!) {
		deleteNamespace(input: {namespaceId: $namespaceId}) {
			namespaceId
			message
		}
	}`

	params := GraphQLParams{
		Query:     deleteReq,
		Variables: map[string]interface{}{"namespaceId": nsID},
	}
	resp, err := hc.RunGraphqlQuery(params, true)
	if err != nil {
		return 0, err
	}

	var result struct {
		DeleteNamespace struct {
			NamespaceId uint64 `json:"namespaceId"`
			Message     string `json:"message"`
		}
	}

	if err := json.Unmarshal(resp, &result); err != nil {
		return 0, errors.Wrap(err, "error unmarshalling CreateNamespaceWithRetry() response")
	}
	if strings.Contains(result.DeleteNamespace.Message, "Deleted namespace successfully") {
		return result.DeleteNamespace.NamespaceId, nil
	}
	return 0, errors.New(result.DeleteNamespace.Message)
}

func (hc *HTTPClient) ListNamespaces() ([]uint64, error) {
	const listNss = `{ state {
		namespaces
	  }
	}`

	params := GraphQLParams{Query: listNss}
	resp, err := hc.RunGraphqlQuery(params, true)
	if err != nil {
		return nil, err
	}

	var result struct {
		State struct {
			Namespaces []uint64 `json:"namespaces"`
		} `json:"state"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.Wrap(err, "error unmarshalling response")
	}

	return result.State.Namespaces, nil
}
