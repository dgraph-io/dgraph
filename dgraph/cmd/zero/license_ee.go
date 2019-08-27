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
	"bytes"
	"io/ioutil"
	"math"
	"net/http"
	"time"

	"github.com/dgraph-io/badger/y"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	humanize "github.com/dustin/go-humanize"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/glog"
	"golang.org/x/net/context"
)

// proposeTrialLicense proposes an enterprise license valid for 30 days.
func (n *node) proposeTrialLicense() error {
	// Apply enterprise license valid for 30 days from now.
	proposal := &pb.ZeroProposal{
		License: &pb.License{
			MaxNodes: math.MaxUint64,
			ExpiryTs: time.Now().Add(humanize.Month).Unix(),
		},
	}
	err := n.proposeAndWait(context.Background(), proposal)
	if err != nil {
		return err

	}
	glog.Infof("Enterprise state proposed to the cluster: %v", proposal)
	return nil
}

func (s *Server) license() *pb.License {
	s.RLock()
	defer s.RUnlock()
	return proto.Clone(s.state.GetLicense()).(*pb.License)
}

func (s *Server) expireLicense() {
	s.Lock()
	defer s.Unlock()
	s.state.License.Enabled = false
}

// periodically checks the validity of the enterprise license and
// 1. Sets license.Enabled to false in membership state if license has expired.
// 2. Prints out warning once every day a week before the license is set to expire.
func (n *node) updateEnterpriseState(closer *y.Closer) {
	defer closer.Done()

	interval := 5 * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	intervalsInDay := int64(24*time.Hour) / int64(interval)
	var counter int64
	for {
		select {
		case <-ticker.C:
			counter++
			license := n.server.license()
			if !license.GetEnabled() {
				continue
			}

			expiry := time.Unix(license.GetExpiryTs(), 0)
			timeToExpire := expiry.Sub(time.Now())
			// We only want to print this log once a day.
			if counter%intervalsInDay == 0 && timeToExpire > 0 && timeToExpire < humanize.Week {
				glog.Warningf("Enterprise license is going to expire in %s.", humanize.Time(expiry))
			}

			active := time.Now().Before(expiry)
			if !active {
				n.server.expireLicense()
				glog.Warningf("Enterprise license has expired and enterprise features would be " +
					"disabled now. Talk to us at contact@dgraph.io to get a new license.")
			}
		case <-closer.HasBeenClosed():
			return
		}
	}
}

// applyEnterpriseLicense accepts a PGP message as a POST request body, verifies that it was
// signed using our private key and applies the license which has maxNodes and Expiry to the
// cluster.
func (st *state) applyEnterpriseLicense(w http.ResponseWriter, r *http.Request) {
	x.AddCorsHeaders(w)
	if r.Method == "OPTIONS" {
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusBadRequest)
		x.SetStatus(w, x.ErrorInvalidMethod, "Invalid method")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		x.SetStatus(w, x.ErrorInvalidRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := st.zero.applyLicense(ctx, bytes.NewReader(b)); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		x.SetStatus(w, x.ErrorInvalidRequest, err.Error())
		return
	}
	x.SetStatus(w, x.Success, "Done")
}
