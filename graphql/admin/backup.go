/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
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

package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime/pprof"
	"time"

	"github.com/dgraph-io/dgraph/graphql/resolve"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/golang/glog"
)

type backupInput struct {
	DestinationFields
	ForceFull bool
}

func resolveBackup(ctx context.Context, m schema.Mutation) (*resolve.Resolved, bool) {
	go func() {
		for i := 0; ; i++ {
			<-time.After(time.Second * 1)
			f, err := os.OpenFile(fmt.Sprintf("/data/heap_%d", i), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				panic(err)
			}
			fmt.Printf("-----------------------------------------: %d\n", i)
			pprof.Lookup("heap").WriteTo(f, 0)
		}
	}()

	glog.Info("Got backup request")

	input, err := getBackupInput(m)
	if err != nil {
		return resolve.EmptyResult(m, err), false
	}

	err = worker.ProcessBackupRequest(context.Background(), &pb.BackupRequest{
		Destination:  input.Destination,
		AccessKey:    input.AccessKey,
		SecretKey:    input.SecretKey,
		SessionToken: input.SessionToken,
		Anonymous:    input.Anonymous,
	}, input.ForceFull)

	if err != nil {
		return resolve.EmptyResult(m, err), false
	}

	return &resolve.Resolved{
		Data:  map[string]interface{}{m.Name(): response("Success", "Backup completed.")},
		Field: m,
	}, true
}

func getBackupInput(m schema.Mutation) (*backupInput, error) {
	inputArg := m.ArgValue(schema.InputArgName)
	inputByts, err := json.Marshal(inputArg)
	if err != nil {
		return nil, schema.GQLWrapf(err, "couldn't get input argument")
	}

	var input backupInput
	err = json.Unmarshal(inputByts, &input)
	return &input, schema.GQLWrapf(err, "couldn't get input argument")
}
