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

	"github.com/dgraph-io/dgraph/graphql/resolve"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/worker"
)

type restoreStatus struct {
	Status string   `json:"status,omitempty"`
	Errors []string `json:"errors,omitempty"`
}

func resolveRestoreStatus(ctx context.Context, q schema.Query) *resolve.Resolved {
	restoreId := q.ArgValue("restoreId").(string)
	status, err := worker.ProcessRestoreStatus(ctx, restoreId)
	if err != nil {
		return &resolve.Resolved{
			Data: map[string]interface{}{q.Name(): map[string]interface{}{
				"status": "UNKNOWN",
			}},
			Field: q,
		}
	}
	convertedStatus := convertStatus(status)

	b, err := json.Marshal(convertedStatus)
	if err != nil {
		return &resolve.Resolved{
			Data: map[string]interface{}{q.Name(): map[string]interface{}{
				"status": "UNKNOWN",
			}},
			Field: q,
		}
	}
	result := make(map[string]interface{})
	err = json.Unmarshal(b, &result)
	if err != nil {
		return &resolve.Resolved{
			Data: map[string]interface{}{q.Name(): map[string]interface{}{
				"status": "UNKNOWN",
			}},
			Field: q,
		}
	}

	return &resolve.Resolved{
		Data:  map[string]interface{}{q.Name(): result},
		Field: q,
	}
}

func convertStatus(status *worker.RestoreStatus) *restoreStatus {
	if status == nil {
		return nil
	}
	res := &restoreStatus{
		Status: status.Status,
		Errors: make([]string, len(status.Errors)),
	}
	for i, err := range status.Errors {
		res.Errors[i] = err.Error()
	}
	return res
}
