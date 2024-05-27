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
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/dgraph-io/dgo/v230"
	"github.com/dgraph-io/dgo/v230/protos/api"
	"github.com/dgraph-io/dgraph/dgraphapi"
	"github.com/dgraph-io/dgraph/x"
)

var (
	CheckUpgrade x.SubCommand
)

const (
	alphaGrpc = "grpc_port"
	alphaHttp = "http_port"
	dgUser    = "dgUser"
	password  = "password"
	namespace = "namespace"
)

type commandInput struct {
	alphaGrpc string
	alphaHttp string
	dgUser    string
	password  string
	namespace uint64
}

type aclNode struct {
	UID        string   `json:"uid"`
	DgraphXID  string   `json:"dgraph.xid"`
	DgraphType []string `json:"dgraph.type"`
}

func setupClients(alphaGrpc, alphaHttp string) (*dgo.Dgraph, *dgraphapi.HTTPClient, error) {
	d, err := grpc.Dial(alphaGrpc, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, errors.Wrapf(err, "while dialing gRPC server")
	}

	httpClient, err := dgraphapi.GetHttpClient(alphaHttp, "")
	if err != nil {
		return nil, nil, errors.Wrapf(err, "while getting HTTP client")
	}
	return dgo.NewDgraphClient(api.NewDgraphClient(d)), httpClient, nil
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

func queryDuplicateNodes(ctx context.Context, dg *dgo.Dgraph) ([3]map[string][]string, error) {
	query := `{ 
		nodes(func: has(dgraph.xid)) {
			       uid
			       dgraph.xid
                   dgraph.type
		        }
	}`

	resp, err := dg.NewTxn().Query(ctx, query)
	if err != nil {
		return [3]map[string][]string{}, errors.Wrapf(err, "while querying dgraph for duplicate nodes")
	}

	type Nodes struct {
		Nodes []aclNode `json:"nodes"`
	}
	var result Nodes
	if err := json.Unmarshal(resp.Json, &result); err != nil {
		return [3]map[string][]string{}, errors.Wrapf(err, "while unmarshalling response: %v", string(resp.Json))

	}
	return findDuplicateNodes(result.Nodes), nil
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
	flag.String(alphaGrpc, "127.0.0.1:9080", "Dgraph Alpha gRPC server address")
	flag.String(alphaHttp, "127.0.0.1:8080", "Dgraph Alpha Http server address")
	flag.String(namespace, "0", "Namespace to check for duplicate nodes")
	flag.String(dgUser, "groot", "Username of the namespace's user")
	flag.String(password, "password", "Password of the namespace's user")
}

func run() {
	if err := checkUpgrade(); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

func checkUpgrade() error {
	fmt.Println("Running check-upgrade tool")

	cmdInput := parseInput()
	gc, hc, err := setupClients(cmdInput.alphaGrpc, cmdInput.alphaHttp)
	if err != nil {
		return errors.Wrapf(err, "while setting up clients")
	}

	if err = hc.LoginIntoNamespace(cmdInput.dgUser, cmdInput.password, cmdInput.namespace); err != nil {
		return errors.Wrapf(err, "while logging into namespace: %v", x.GalaxyNamespace)
	}

	ctx := context.Background()
	if err := gc.LoginIntoNamespace(ctx, cmdInput.dgUser, cmdInput.password, cmdInput.namespace); err != nil {
		return errors.Wrapf(err, "while logging into namespace: %v", cmdInput.namespace)
	}

	duplicates, err := queryDuplicateNodes(ctx, gc)
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

	return nil
}

func parseInput() *commandInput {
	return &commandInput{alphaGrpc: CheckUpgrade.Conf.GetString(alphaGrpc),
		alphaHttp: CheckUpgrade.Conf.GetString(alphaHttp), dgUser: CheckUpgrade.Conf.GetString(dgUser),
		password: CheckUpgrade.Conf.GetString(password), namespace: CheckUpgrade.Conf.GetUint64(namespace)}
}
