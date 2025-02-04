/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package dgraphapi

import (
	"encoding/json"
	"time"

	"github.com/pkg/errors"
)

func (hc *HTTPClient) GetCurrentSnapshotTs(group uint64) (uint64, error) {
	snapTsRequest := `query {
		state {
		  groups {
			id
			snapshotTs
		  }
		}
	  }`
	params := GraphQLParams{
		Query: snapTsRequest,
	}
	resp, err := hc.RunGraphqlQuery(params, true)
	if err != nil {
		return 0, err
	}

	var stateResp struct {
		State struct {
			Groups []struct {
				SnapshotTs uint64
			}
		}
	}

	err = json.Unmarshal(resp, &stateResp)
	if err != nil {
		return 0, err
	}

	return stateResp.State.Groups[group-1].SnapshotTs, nil
}

func (hc *HTTPClient) WaitForSnapshot(group, prevSnapshotTs uint64) (uint64, error) {

	for i := 1; i <= 100; i++ {
		currentSnapshotTs, err := hc.GetCurrentSnapshotTs(group)
		if err != nil {
			return 0, errors.Wrapf(err, "error while getting current snapshot timestamp: %v", err)
		}
		if currentSnapshotTs > prevSnapshotTs {
			return currentSnapshotTs, nil
		}

		time.Sleep(time.Second)
	}
	return 0, errors.New("timeout excedded")
}
