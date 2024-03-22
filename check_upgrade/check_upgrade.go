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
	"slices"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/dgraph-io/dgo/v230"
	"github.com/dgraph-io/dgo/v230/protos/api"
	"github.com/dgraph-io/dgraph/dgraphtest"
	"github.com/dgraph-io/dgraph/x"
)

var (
	CheckUpgrade x.SubCommand
)

const (
	alphaGrpc = "grpc_port"
	alphaHttp = "http_port"
)

type commandInput struct {
	alphaGrpc string
	alphaHttp string
}

type Node struct {
	UID        string   `json:"uid"`
	DgraphXID  string   `json:"dgraph.xid"`
	DgraphType []string `json:"dgraph.type"`
}

func setupClients(alphaGrpc, alphaHttp string) (*dgo.Dgraph, dgraphtest.HTTPClient, error) {
	d, err := grpc.Dial(alphaGrpc, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, dgraphtest.HTTPClient{}, errors.Wrapf(err, "while dialing gRPC server")
	}

	return dgo.NewDgraphClient(api.NewDgraphClient(d)), dgraphtest.HTTPClient{
		AdminURL:   alphaHttp + "/admin",
		GraphqlURL: alphaHttp + "/graphql",
	}, nil
}

func findDuplicateNodes(users []Node) []Node {
	duplicateUsers := make([]Node, 0)
	for _, i := range users {
		for _, j := range users {
			if i.DgraphXID == j.DgraphXID && i.UID != j.UID {
				duplicateUsers = append(duplicateUsers, i)
			}

		}

	}

	return duplicateUsers
}

func getDuplicateNodes(ctx context.Context, dg *dgo.Dgraph) ([]Node, error) {
	query := `{ 
		nodes(func: has(dgraph.xid)) {
			       uid
			       dgraph.xid
                   dgraph.type
		        }
	}`

	resp, err := dg.NewTxn().Query(ctx, query)
	if err != nil {
		return []Node{}, errors.Wrapf(err, "while querying dgraph for duplicate nodes")
	}

	type Nodes struct {
		Nodes []Node `json:"nodes"`
	}
	var result Nodes
	err = json.Unmarshal([]byte(resp.Json), &result)
	if err != nil {
		return []Node{}, errors.Wrapf(err, "while unmarshalling response: %v", string(resp.Json))
	}

	duplicates := findDuplicateNodes(result.Nodes)

	return duplicates, nil
}

func printDuplicates(entityType string, ns uint64, nodes []Node) {
	if len(nodes) > 0 {
		fmt.Printf("Found duplicate %ss in namespace: #%v\n", entityType, ns)
		for _, node := range nodes {
			fmt.Printf("dgraph.xid %v , Uid: %v\n", node.DgraphXID, node.UID)
		}
	}
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
	flag.String(alphaGrpc, "127.0.0.1:9080",
		"Dgraph Alpha gRPC server address")

	flag.String(alphaHttp, "http://127.0.0.1:8080", "Draph Alpha HTTP(S) endpoint.")
}

func duplicateDeleteMutation(nodes []Node) {
	if len(nodes) > 0 {
		var deleteMutation strings.Builder
		deleteMutation.WriteString("{\n")
		deleteMutation.WriteString(" delete {\n")
		for _, node := range nodes {
			deleteMutation.WriteString(fmt.Sprintf("<%v> <%v> \"%v\" .\n", node.UID, "dgraph.xid", node.DgraphXID))
		}
		deleteMutation.WriteString(" }\n")
		deleteMutation.WriteString(" }\n")

		fmt.Printf("\ndelete duplicate nodes using following mutation : \n%v", deleteMutation.String())
	}
}

func run() {
	fmt.Println("Running check-upgrade tool")

	cmdInput := parseInput()

	gc, hc, err := setupClients(cmdInput.alphaGrpc, cmdInput.alphaHttp)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}

	hc.LoginIntoNamespace(dgraphtest.DefaultUser, dgraphtest.DefaultPassword, x.GalaxyNamespace)
	if err != nil {
		fmt.Fprintln(os.Stderr, errors.Wrapf(err, "while logging into namespace: %v", x.GalaxyNamespace))
		return
	}

	namespaces, err := hc.ListNamespaces()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}

	ctx := context.Background()

	for _, ns := range namespaces {
		if err := gc.LoginIntoNamespace(ctx, dgraphtest.DefaultUser, dgraphtest.DefaultPassword, ns); err != nil {
			fmt.Fprintln(os.Stderr, errors.Wrapf(err, "while logging into namespace: %v", ns))
			return
		}

		duplicates, err := getDuplicateNodes(ctx, gc)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return
		}

		var duplicateUsers []Node
		var duplicateGroups []Node

		for _, node := range duplicates {
			if slices.Contains(node.DgraphType, "dgraph.type.User") {
				duplicateUsers = append(duplicateUsers, node)
			} else if slices.Contains(node.DgraphType, "dgraph.type.Group") {
				duplicateGroups = append(duplicateGroups, node)
			}
		}

		printDuplicates("user", ns, duplicateUsers)
		printDuplicates("group", ns, duplicateGroups)
		duplicateDeleteMutation(duplicateUsers)
		duplicateDeleteMutation(duplicateGroups)
	}
}

func parseInput() *commandInput {
	return &commandInput{alphaGrpc: CheckUpgrade.Conf.GetString(alphaGrpc), alphaHttp: CheckUpgrade.Conf.GetString(alphaHttp)}
}
