/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package dgraphtest

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// ZeroMember is a minimal view of a Zero member as reported by the /state
// endpoint. Only the fields relevant to address reconciliation are included.
type ZeroMember struct {
	Addr   string `json:"addr"`
	Leader bool   `json:"leader"`
}

// ZeroState is the subset of Zero's /state response that we care about for
// testing. It intentionally mirrors only the fields used by tests, keeping
// unmarshal resilient to unrelated schema changes.
type ZeroState struct {
	Zeros map[string]ZeroMember `json:"zeros"`
}

// GetZeroStateURL returns the full HTTP URL of a Zero's /state endpoint.
func (c *LocalCluster) GetZeroStateURL(id int) (string, error) {
	if id >= c.conf.numZeros {
		return "", fmt.Errorf("invalid id of zero: %v", id)
	}
	pubPort, err := publicPort(c.dcli, c.zeros[id], zeroHttpPort)
	if err != nil {
		return "", err
	}
	return "http://0.0.0.0:" + pubPort + "/state", nil
}

// GetZeroState queries the /state endpoint on the specified Zero and returns
// the parsed membership snapshot.
func (c *LocalCluster) GetZeroState(id int) (*ZeroState, error) {
	stateURL, err := c.GetZeroStateURL(id)
	if err != nil {
		return nil, err
	}

	resp, err := http.Get(stateURL)
	if err != nil {
		return nil, errors.Wrapf(err, "GET %s", stateURL)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "reading /state body")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("/state HTTP %d: %s", resp.StatusCode, body)
	}

	var state ZeroState
	if err := json.Unmarshal(body, &state); err != nil {
		return nil, errors.Wrapf(err, "unmarshal /state (body: %s)", string(body))
	}
	return &state, nil
}

// ZeroLocator identifies a Zero by its container index and Raft ID.
type ZeroLocator struct {
	ContainerIdx int
	RaftID       string
	Addr         string
}

// GetZeroLeader returns the locator of the current Zero leader, as reported by
// the Zero at queryIdx.
func (c *LocalCluster) GetZeroLeader(queryIdx int) (*ZeroLocator, error) {
	return c.findZero(queryIdx, true)
}

// GetZeroFollower returns the locator of any Zero that is not the leader, as
// reported by the Zero at queryIdx.
func (c *LocalCluster) GetZeroFollower(queryIdx int) (*ZeroLocator, error) {
	return c.findZero(queryIdx, false)
}

// GetZeroFollowers returns the locators of all non-leader Zeros, as reported
// by the Zero at queryIdx.
func (c *LocalCluster) GetZeroFollowers(queryIdx int) ([]ZeroLocator, error) {
	state, err := c.GetZeroState(queryIdx)
	if err != nil {
		return nil, err
	}
	var followers []ZeroLocator
	for id, z := range state.Zeros {
		if z.Leader {
			continue
		}
		idx, ok := containerIdxFromAddr(z.Addr)
		if !ok {
			return nil, fmt.Errorf("cannot map follower addr %q to container index", z.Addr)
		}
		followers = append(followers, ZeroLocator{ContainerIdx: idx, RaftID: id, Addr: z.Addr})
	}
	return followers, nil
}

func (c *LocalCluster) findZero(queryIdx int, wantLeader bool) (*ZeroLocator, error) {
	state, err := c.GetZeroState(queryIdx)
	if err != nil {
		return nil, err
	}
	for id, z := range state.Zeros {
		if z.Leader != wantLeader {
			continue
		}
		idx, ok := containerIdxFromAddr(z.Addr)
		if !ok {
			return nil, fmt.Errorf("cannot map addr %q to container index", z.Addr)
		}
		return &ZeroLocator{ContainerIdx: idx, RaftID: id, Addr: z.Addr}, nil
	}
	role := "follower"
	if wantLeader {
		role = "leader"
	}
	return nil, fmt.Errorf("no %s found in /state from zero%d", role, queryIdx)
}

// WaitForZeroAddress polls /state until the member identified by raftID has
// wantAddr. It first tries queryIdx; if that node's /state is unavailable
// (e.g. it temporarily lost quorum during a leadership change) it falls back
// to any other Zero in the cluster. The last observed address and an error
// are returned if the target is not reached within timeout.
func (c *LocalCluster) WaitForZeroAddress(queryIdx int, raftID, wantAddr string,
	timeout, poll time.Duration) (string, error) {

	deadline := time.Now().Add(timeout)
	var lastAddr string
	var lastErr error
	for time.Now().Before(deadline) {
		// Try queryIdx first, fall back to other nodes when quorum is transiently lost.
		for _, idx := range append([]int{queryIdx}, otherZeroIdxs(queryIdx, c.conf.numZeros)...) {
			state, err := c.GetZeroState(idx)
			if err != nil {
				lastErr = err
				continue
			}
			z, ok := state.Zeros[raftID]
			if !ok {
				lastErr = fmt.Errorf("raft id %s not present in /state", raftID)
				continue
			}
			lastAddr = z.Addr
			if lastAddr == wantAddr {
				return lastAddr, nil
			}
			break
		}
		time.Sleep(poll)
	}
	if lastErr != nil {
		return lastAddr, errors.Wrapf(lastErr,
			"timed out waiting for zero %s addr=%q on zero%d (last seen %q)",
			raftID, wantAddr, queryIdx, lastAddr)
	}
	return lastAddr, fmt.Errorf(
		"timed out waiting for zero %s addr=%q on zero%d (last seen %q)",
		raftID, wantAddr, queryIdx, lastAddr)
}

// otherZeroIdxs returns all zero indices except exclude, preserving order.
func otherZeroIdxs(exclude, numZeros int) []int {
	var out []int
	for i := range numZeros {
		if i != exclude {
			out = append(out, i)
		}
	}
	return out
}

// ChangeZeroAddress reconfigures the Zero at id with a new --my flag that
// points to its Docker container name (a valid DNS alias on the cluster
// network). The Zero is stopped, recreated with its WAL preserved, and
// restarted. The new address is returned so callers can assert convergence.
func (c *LocalCluster) ChangeZeroAddress(id int) (string, error) {
	if err := c.StopZero(id); err != nil {
		return "", err
	}
	containerName, err := c.GetZeroContainerName(id)
	if err != nil {
		return "", err
	}
	newAddr := containerName + ":" + zeroGrpcPort
	if err := c.SetZeroMyAddr(id, newAddr); err != nil {
		return "", err
	}
	if err := c.RecreateZero(id); err != nil {
		return "", err
	}
	if err := c.StartZero(id); err != nil {
		return "", err
	}
	return newAddr, nil
}

// containerIdxFromAddr extracts the numeric suffix from the "zeroN" segment
// of a member's address and returns it as the container index used by tests.
func containerIdxFromAddr(addr string) (int, bool) {
	for i := range maxContainerIdxScan {
		if strings.Contains(addr, fmt.Sprintf("zero%d", i)) {
			return i, true
		}
	}
	return -1, false
}

// maxContainerIdxScan bounds the zero N scan in containerIdxFromAddr. It is
// larger than any realistic test topology.
const maxContainerIdxScan = 32

// GetZeroLogs returns the full container stdout/stderr log for the Zero at id.
func (c *LocalCluster) GetZeroLogs(id int) (string, error) {
	if id >= c.conf.numZeros {
		return "", fmt.Errorf("invalid id of zero: %v", id)
	}
	return c.getLogs(c.zeros[id].containerID)
}

// WaitForZeroLog polls the Zero's container log until it contains substr, or
// the timeout expires. It is the log-based equivalent of WaitForZeroAddress
// and lets tests react to concrete events (e.g. a Raft proposal being applied)
// instead of sleeping for an arbitrary duration.
func (c *LocalCluster) WaitForZeroLog(id int, substr string, timeout, poll time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		logs, err := c.GetZeroLogs(id)
		if err != nil {
			lastErr = err
		} else if strings.Contains(logs, substr) {
			return nil
		}
		time.Sleep(poll)
	}
	if lastErr != nil {
		return errors.Wrapf(lastErr, "timed out waiting for log substring %q on zero%d",
			substr, id)
	}
	return fmt.Errorf("timed out waiting for log substring %q on zero%d", substr, id)
}

// ZeroLogContains reports whether the Zero's current log output contains substr.
func (c *LocalCluster) ZeroLogContains(id int, substr string) (bool, error) {
	logs, err := c.GetZeroLogs(id)
	if err != nil {
		return false, err
	}
	return strings.Contains(logs, substr), nil
}

// WaitForAnyZeroLog polls all Zero containers until any one of them contains
// substr, or the timeout expires. Use this when a log marker may appear on any
// node (e.g. after a leadership change caused by the test scenario itself).
func (c *LocalCluster) WaitForAnyZeroLog(substr string, timeout, poll time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for i := range c.conf.numZeros {
			logs, err := c.GetZeroLogs(i)
			if err != nil {
				continue
			}
			if strings.Contains(logs, substr) {
				return nil
			}
		}
		time.Sleep(poll)
	}
	return fmt.Errorf("timed out waiting for log substring %q on any zero", substr)
}
