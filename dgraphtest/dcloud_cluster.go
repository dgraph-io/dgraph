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
	"fmt"
	"os"
	"strconv"

	"github.com/pkg/errors"
	"google.golang.org/grpc"

	"github.com/dgraph-io/dgo/v210"
	"github.com/dgraph-io/dgo/v210/protos/api"
)

type DCloudCluster struct {
	url   string
	token string

	conn   *grpc.ClientConn
	client *dgo.Dgraph
}

func NewDCloudCluster() (DCloudCluster, error) {
	url := os.Getenv("TEST_DGRAPH_CLOUD_CLUSTER_URL")
	token := os.Getenv("TEST_DGRAPH_CLOUD_CLUSTER_TOKEN")
	if url == "" || token == "" {
		return DCloudCluster{}, errors.New("cloud cluster params needed in env")
	}

	done := false
	conn, err := dgo.DialCloud(url, token)
	if err != nil {
		return DCloudCluster{}, errors.Wrap(err, "error creating dgraph client")
	}
	defer func() {
		if !done {
			if err := conn.Close(); err != nil {
				log(nil, "error closing connection: %v", err)
			}
		}
	}()

	client := dgo.NewDgraphClient(api.NewDgraphClient(conn))
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	if err := client.Login(ctx, defaultUser, defaultPassowrd); err != nil {
		return DCloudCluster{}, errors.Wrap(err, "error during login")
	}

	done = true
	return DCloudCluster{
		url:    url,
		token:  token,
		conn:   conn,
		client: client,
	}, nil
}

func (c DCloudCluster) Cleanup() {
	if err := c.conn.Close(); err != nil {
		log(nil, "error closing connection: %v", err)
	}
}

func (c DCloudCluster) Client() *dgo.Dgraph {
	return c.client
}

// AssignUids moves the max assigned UIDs by the given number.
// Note that we this performs dropall after moving the max assigned.
func (c DCloudCluster) AssignUids(num uint64) error {
	// in Dgraph cloud, we can't talk to zero. Therefore, what we instead do
	// is keep doing mutations until the cluster has assigned those many new UIDs

	genData := func() []byte {
		var rdfs bytes.Buffer
		_, _ = rdfs.WriteString("_:root <test_cloud> \"root\" .\n")
		for i := 0; i < 1000; i++ {
			rdfs.WriteString(fmt.Sprintf("_:%v <test_cloud> \"0\" .\n", i))
		}
		return rdfs.Bytes()
	}

	var prev uint64
	for i := uint64(0); i < num; {
		log(nil, "performing mutation for AssignUID: assigned %v UIDs", i)
		mu := &api.Mutation{SetNquads: genData(), CommitNow: true}

		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		resp, err := c.client.NewTxn().Mutate(ctx, mu)
		cancel()
		if err != nil {
			return errors.Wrap(err, "error in mutation during AssignUID")
		}

		var max uint64
		for _, uidStr := range resp.Uids {
			uid, err := strconv.ParseUint(uidStr, 0, 64)
			if err != nil {
				return errors.Wrap(err, "error parsing mutation resp during AssignUID")
			}
			if uid > max {
				max = uid
			}
		}

		if prev == 0 {
			i = 1000
			prev = max
			continue
		}
		if max-prev == 0 {
			return errors.New("mutations did not create new UIDs during AssignUID")
		}

		i += max - prev
		prev = max
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	if err := c.client.Alter(ctx, &api.Operation{DropAll: true}); err != nil {
		return errors.Wrap(err, "error in DropAll during AssignUID")
	}

	return nil
}
