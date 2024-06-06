/*
 * Copyright 2024 Dgraph Labs, Inc. and Contributors
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

package checkupgrade

import (
	"crypto"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/dgraph-io/dgraph/dgraphapi"
	"github.com/dgraph-io/dgraph/x"
)

var (
	CheckUpgrade x.SubCommand
)

const (
	alphaHttp            = "http_port"
	dgUser               = "dgUser"
	password             = "password"
	namespace            = "namespace"
	aclSecretKeyFilePath = "aclSecretKeyFilePath"
	jwtAlg               = "jwt-alg"

	guardianGroup = "guardians"
)

type commandInput struct {
	alphaHttp            string
	dgUser               string
	password             string
	namespace            uint64
	aclSecretKeyFilePath string
	jwtAlg               string
}

type aclNode struct {
	UID        string   `json:"uid"`
	DgraphXID  string   `json:"dgraph.xid"`
	DgraphType []string `json:"dgraph.type"`
}

func parseJWTKey(alg jwt.SigningMethod, key x.Sensitive) (interface{}, interface{}, error) {
	switch {
	case strings.HasPrefix(alg.Alg(), "HS"):
		return key, key, nil

	case strings.HasPrefix(alg.Alg(), "ES"):
		pk, err := jwt.ParseECPrivateKeyFromPEM(key)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "error parsing ACL key as ECDSA private key")
		}
		return pk, &pk.PublicKey, nil

	case strings.HasPrefix(alg.Alg(), "RS") || strings.HasPrefix(alg.Alg(), "PS"):
		pk, err := jwt.ParseRSAPrivateKeyFromPEM(key)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "error parsing ACL key as RSA private key")
		}
		return pk, &pk.PublicKey, nil

	case alg.Alg() == "EdDSA":
		pk, err := jwt.ParseEdPrivateKeyFromPEM(key)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "error parsing ACL key as EdDSA private key")
		}
		return pk.(crypto.Signer), pk.(ed25519.PrivateKey).Public(), nil

	default:
		return nil, nil, errors.Errorf("unsupported signing algorithm: %v", alg.Alg())
	}
}

func getAccessJwt(userId string, group string, namespace uint64, aclSecretFile string,
	algStr string) (string, error) {
	aclKey, err := os.ReadFile(aclSecretFile)
	if err != nil {
		return "", fmt.Errorf("error reading ACL secret key from file: %s: %s", aclSecretFile, err)
	}

	var aclAlg jwt.SigningMethod
	var privKey interface{}
	if aclKey != nil {
		aclAlg = jwt.GetSigningMethod(algStr)
		if aclAlg == nil {
			return "", fmt.Errorf("unsupported jwt signing algorithm for acl: %v", algStr)
		}
		privKey, _, err = parseJWTKey(aclAlg, aclKey)
		if err != nil {
			return "", err
		}
	}

	token := jwt.NewWithClaims(aclAlg, jwt.MapClaims{
		"userid":    userId,
		"groups":    []string{group},
		"namespace": namespace,
		"exp":       time.Now().Add(time.Hour).Unix(),
	})

	jwtString, err := token.SignedString(x.MaybeKeyToBytes(privKey))
	if err != nil {
		return "", errors.Errorf("unable to encode jwt to string: %v", err)
	}
	return jwtString, nil
}

func setupClient(alphaHttp string) (*dgraphapi.HTTPClient, error) {
	httpClient, err := dgraphapi.GetHttpClient(alphaHttp, "")
	if err != nil {
		return nil, errors.Wrapf(err, "while getting HTTP client")
	}
	return httpClient, nil
}

func contains(slice []string, value string) bool {
	for _, v := range slice {
		if v == value {
			return true
		}
	}
	return false
}

func findDuplicateNodes(aclNodes []aclNode) [3]map[string][]string {
	du := make(map[string][]string)
	dg := make(map[string][]string)
	dug := make(map[string][]string)

	for i, node1 := range aclNodes {
		for j := i + 1; j < len(aclNodes); j++ {
			node2 := aclNodes[j]
			if node1.DgraphXID == node2.DgraphXID {
				if node1.DgraphType[0] == "dgraph.type.User" && node1.DgraphType[0] == node2.DgraphType[0] {
					filterAndRecordDuplicates(du, node1, node2)
				} else if node1.DgraphType[0] == "dgraph.type.Group" && node1.DgraphType[0] == node2.DgraphType[0] {
					filterAndRecordDuplicates(dg, node1, node2)
				} else {
					filterAndRecordDuplicates(dug, node1, node2)
				}
			}
		}
	}

	return [3]map[string][]string{
		du, dg, dug,
	}
}

func filterAndRecordDuplicates(du map[string][]string, node1 aclNode, node2 aclNode) {
	if _, exists := du[node1.DgraphXID]; !exists {
		du[node1.DgraphXID] = []string{}
	}
	if !contains(du[node1.DgraphXID], node1.UID) {
		du[node1.DgraphXID] = append(du[node1.DgraphXID], node1.UID)
	}
	if !contains(du[node1.DgraphXID], node2.UID) {
		du[node1.DgraphXID] = append(du[node1.DgraphXID], node2.UID)
	}
}

func queryDuplicateNodes(hc *dgraphapi.HTTPClient) ([3]map[string][]string, error) {
	query := `{ 
		nodes(func: has(dgraph.xid)) {
			       uid
			       dgraph.xid
                   dgraph.type
		        }
	}`

	resp, err := hc.PostDqlQuery(query)

	fmt.Println("resp: ", string(resp))
	if err != nil {
		return [3]map[string][]string{}, errors.Wrapf(err, "while querying dgraph for duplicate nodes")
	}

	type Nodes struct {
		Nodes []aclNode `json:"nodes"`
	}
	type Response struct {
		Data Nodes `json:"data"`
	}
	var result Response
	if err := json.Unmarshal(resp, &result); err != nil {
		return [3]map[string][]string{}, errors.Wrapf(err, "while unmarshalling response: %v", string(resp))

	}
	return findDuplicateNodes(result.Data.Nodes), nil
}

func printDuplicates(entityType string, ns uint64, nodesmap map[string][]string) {
	if len(nodesmap) == 0 {
		return
	}

	fmt.Printf("Found duplicate %ss in namespace: #%v\n", entityType, ns)
	for key, node := range nodesmap {
		fmt.Printf("dgraph.xid %v , Uids: %v\n", key, node)
	}
	fmt.Println("")
}

func init() {
	CheckUpgrade.Cmd = &cobra.Command{
		Use:   "checkupgrade",
		Short: "Run the checkupgrade tool",
		Long:  "The checkupgrade tool is used to check for duplicate dgraph.xid's in the Dgraph database before upgrade.",
		Run: func(cmd *cobra.Command, args []string) {
			run()
		},
		Annotations: map[string]string{"group": "tool"},
	}
	CheckUpgrade.Cmd.SetHelpTemplate(x.NonRootTemplate)
	flag := CheckUpgrade.Cmd.Flags()
	flag.String(alphaHttp, "127.0.0.1:8080", "Dgraph Alpha Http server address")
	flag.String(namespace, "", "Namespace to check for duplicate nodes")
	flag.String(dgUser, "groot", "Username of the namespace's user")
	flag.String(password, "password", "Password of the namespace's user")
	flag.String(aclSecretKeyFilePath, "", "path of file that stores secret key or private key, which is used to sign the ACL JWT")
	flag.String(jwtAlg, "HS256", "JWT signing algorithm")
}

func run() {
	if err := checkUpgrade(); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

func checkUpgrade() error {
	fmt.Println("Running check-upgrade tool")

	cmdInput := parseInput()
	var accessJwt string
	var err error
	if cmdInput.aclSecretKeyFilePath != "" {
		accessJwt, err = getAccessJwt(dgraphapi.DefaultUser, guardianGroup, 0, cmdInput.aclSecretKeyFilePath,
			cmdInput.jwtAlg)
		if err != nil {
			return errors.Wrapf(err, "while getting access jwt token")
		}
	}
	hc, err := setupClient(cmdInput.alphaHttp)
	if err != nil {
		return errors.Wrapf(err, "while setting up clients")
	}

	hc.AccessJwt = accessJwt

	var namespaces []uint64
	if cmdInput.namespace == 0 {
		namespaces, err = hc.ListNamespaces()
		if err != nil {
			return errors.Wrapf(err, "while lisiting namespaces")
		}
	} else {
		namespaces = append(namespaces, cmdInput.namespace)
	}

	for _, ns := range namespaces {
		if cmdInput.aclSecretKeyFilePath != "" {
			hc.AccessJwt, err = getAccessJwt(dgraphapi.DefaultUser, guardianGroup, ns, cmdInput.aclSecretKeyFilePath,
				cmdInput.jwtAlg)
			if err != nil {
				return errors.Wrapf(err, "while getting access jwt token for namespace %v", ns)
			}
		} else {
			hc.LoginIntoNamespace(cmdInput.dgUser, cmdInput.password, ns)
		}

		duplicates, err := queryDuplicateNodes(hc)
		if err != nil {
			return err
		}
		printDuplicates("user", cmdInput.namespace, duplicates[0])
		// example output:
		//	Found duplicate users in namespace: #0
		// dgraph.xid user1 , Uids: [0x4 0x3]
		printDuplicates("group", cmdInput.namespace, duplicates[1])
		// Found duplicate groups in namespace: #1
		// dgraph.xid group1 , Uids: [0x2714 0x2711]
		printDuplicates("groups and user", cmdInput.namespace, duplicates[2])
		// Found duplicate groups and users in namespace: #0
		// dgraph.xid userGroup1 , Uids: [0x7532 0x7531]

		fmt.Println("To delete duplicate nodes use following mutation: ")
		deleteMut := `
	{
		delete {
			<UID> * * .
		}
	}`
		fmt.Fprint(os.Stderr, deleteMut)

	}

	return nil
}

func parseInput() *commandInput {
	return &commandInput{alphaHttp: CheckUpgrade.Conf.GetString(alphaHttp), dgUser: CheckUpgrade.Conf.GetString(dgUser),
		password: CheckUpgrade.Conf.GetString(password), namespace: CheckUpgrade.Conf.GetUint64(namespace),
		aclSecretKeyFilePath: CheckUpgrade.Conf.GetString(aclSecretKeyFilePath),
		jwtAlg:               CheckUpgrade.Conf.GetString(jwtAlg)}
}
