// +build !oss

/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
 */

package zero

import (
	"math"
	"time"

	"github.com/dgraph-io/dgraph/protos/pb"
	humanize "github.com/dustin/go-humanize"
	"github.com/golang/glog"
	"golang.org/x/net/context"
)

// proposeTrialLicense proposes an enterprise license valid for 30 days.
func (n *node) proposeTrialLicense() {
	// Apply enterprise license valid for 30 days from now.
	proposal := &pb.ZeroProposal{
		License: &pb.License{
			MaxNodes: math.MaxUint64,
			ExpiryTs: time.Now().Add(humanize.Month).Unix(),
		},
	}
	for {
		err := n.proposeAndWait(context.Background(), proposal)
		if err == nil {
			glog.Infof("Enterprise state proposed to the cluster: %v", proposal)
			return
		}
		if err == errInvalidProposal {
			glog.Errorf("invalid proposal error while proposing enteprise state")
			return
		}
		glog.Errorf("While proposing enterprise state: %v. Retrying...", err)
		time.Sleep(3 * time.Second)
	}
}

// updateEnterpriseState periodically checks the validity of the enterprise license
// based on its expiry.
func (s *Server) updateEnterpriseState() {
	s.Lock()
	defer s.Unlock()

	// Return early if license is not enabled. This would happen when user didn't supply us a
	// license file yet.
	if s.state.GetLicense() == nil {
		return
	}

	enabled := s.state.GetLicense().GetEnabled()
	expiry := time.Unix(s.state.License.ExpiryTs, 0)
	s.state.License.Enabled = time.Now().Before(expiry)
	if enabled && !s.state.License.Enabled {
		// License was enabled earlier and has just now been disabled.
		glog.Infof("Enterprise license has expired and enterprise features would be disabled now. " +
			"Talk to us at contact@dgraph.io to get a new license.")
	}
}

// Prints out an info log about the expiry of the license if its about to expire in less than a
// week.
func (s *Server) licenseExpiryWarning() {
	s.RLock()
	defer s.RUnlock()

	if s.state.GetLicense() == nil {
		return
	}
	enabled := s.state.GetLicense().GetEnabled()
	expiry := time.Unix(s.state.License.ExpiryTs, 0)
	timeToExpire := expiry.Sub(time.Now())
	if enabled && timeToExpire > 0 && timeToExpire < humanize.Week {
		glog.Infof("Enterprise license is going to expire in %s.", humanize.Time(expiry))
	}
}
