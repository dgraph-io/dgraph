/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
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
package main

import (
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/z"

	"github.com/stretchr/testify/require"
)

func TestGroupJoin(t *testing.T) {
	// remove all the memebers in group 2
	state, err := z.GetState()
	require.NoError(t, err)
	group, ok := state.Groups["2"]
	if !ok {
		t.Error("group 2 not found")
		return
	}
	if group.Members == nil {
		t.Error("members not found on group 2")
		return
	}
	for id := range group.Members {
		resp, err := http.Get(fmt.Sprintf("http://%s/removeNode?group=2&id=%s", z.SockAddrZeroHttp, id))
		require.NoError(t, err)
		require.NoError(t, z.GetError(resp.Body))
	}

	cmd := exec.Command("docker-compose", "ps")
	stdout, err := cmd.Output()
	require.NoError(t, err)
	lines := strings.Split(string(stdout), "\n")
	// sample output
	// Name                 Command                State                                   Ports
	// ---------------------------------------------------------------------------------------------------------------------------
	// bank-dg0.1   /gobin/dgraph zero -o 100  ...   Up         0.0.0.0:5180->5180/tcp, 0.0.0.0:6180->6180/tcp, 8080/tcp, 9080/tcp
	// bank-dg1     /gobin/dgraph alpha --my=d ...   Up         8080/tcp, 0.0.0.0:8180->8180/tcp, 9080/tcp, 0.0.0.0:9180->9180/tcp
	// bank-dg2     /gobin/dgraph alpha --my=d ...   Up         8080/tcp, 0.0.0.0:8182->8182/tcp, 9080/tcp, 0.0.0.0:9182->9182/tcp
	// bank-dg3     /gobin/dgraph alpha --my=d ...   Up         8080/tcp, 0.0.0.0:8183->8183/tcp, 9080/tcp, 0.0.0.0:9183->9183/tcp
	// bank-dg4     /gobin/dgraph alpha --my=d ...   Up         8080/tcp, 0.0.0.0:8184->8184/tcp, 9080/tcp, 0.0.0.0:9184->9184/tcp
	// bank-dg5     /gobin/dgraph alpha --my=d ...   Up         8080/tcp, 0.0.0.0:8185->8185/tcp, 9080/tcp, 0.0.0.0:9185->9185/tcp
	// bank-dg6     /gobin/dgraph alpha --my=d ...   Exit 255
	// bank-dg7     /gobin/dgraph alpha --my=d ...   Exit 0
	// bank-dg8     /gobin/dgraph alpha --my=d ...   Exit 255
	// bank-dg9     /gobin/dgraph alpha --my=d ...   Up         8080/tcp, 0.0.0.0:8189->8189/tcp, 9080/tcp, 0.0.0.0:9189->9189/tcp

	// ignoring first 3 lines
	for i := 3; i < len(lines); i++ {
		if strings.Contains(lines[i], "Exit") {
			// srejoin the node to the cluster
			cmd := exec.Command("docker-compose", "up", "--force-recreate", "--detach", fmt.Sprintf("dg%d", i-2))
			_, err := cmd.Output()
			require.NoError(t, err)
		}
	}
	// give some time to establish membership
	time.Sleep(time.Second * 4)
	state, err = z.GetState()
	require.NoError(t, err)
	if group, ok := state.Groups["2"]; ok {
		if group.Members != nil {
			if len(group.Members) != 3 {
				t.Errorf("expected member in gorup 2 is 3 but got %d", len(group.Members))
			}
			return
		}
		t.Error("members not found on group 2")
		return
	}
	t.Error("group 2 not found")
}
