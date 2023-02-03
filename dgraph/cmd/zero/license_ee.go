//go:build !oss
// +build !oss

/*
 * Copyright 2022 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
 */

package zero

import (
	"context"
	"io/ioutil"
	"math"
	"net/http"
	"time"

	humanize "github.com/dustin/go-humanize"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/glog"

	"github.com/dgraph-io/dgraph/ee/audit"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
)

// proposeTrialLicense proposes an enterprise license valid for 30 days.
func (n *node) proposeTrialLicense() error {
	// Apply enterprise license valid for 30 days from now.
	proposal := &pb.ZeroProposal{
		License: &pb.License{
			MaxNodes: math.MaxUint64,
			ExpiryTs: time.Now().UTC().Add(humanize.Month).Unix(),
		},
	}
	err := n.proposeAndWait(context.Background(), proposal)
	if err != nil {
		return err

	}
	glog.Infof("Enterprise trial license proposed to the cluster: %v", proposal)
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
func (n *node) updateEnterpriseState(closer *z.Closer) {
	defer closer.Done()

	interval := 5 * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	intervalsInDay := int64(24*time.Hour) / int64(interval)
	var counter int64
	crashLearner := func() {
		if n.RaftContext.IsLearner {
			glog.Errorf("Enterprise License missing or expired. " +
				"Learner nodes need an Enterprise License.")
			// Signal the zero node to stop.
			n.server.closer.Signal()
		}
	}
	for {
		select {
		case <-ticker.C:
			counter++
			license := n.server.license()
			if !license.GetEnabled() {
				crashLearner()
				continue
			}

			expiry := time.Unix(license.GetExpiryTs(), 0).UTC()
			timeToExpire := expiry.Sub(time.Now().UTC())
			// We only want to print this log once a day.
			if counter%intervalsInDay == 0 && timeToExpire > 0 && timeToExpire < humanize.Week {
				glog.Warningf("Your enterprise license will expire in %s. To continue using enterprise "+
					"features after %s, apply a valid license. To get a new license, contact us at "+
					"https://dgraph.io/contact.", humanize.Time(expiry), humanize.Time(expiry))
			}

			active := time.Now().UTC().Before(expiry)
			if !active {
				n.server.expireLicense()
				audit.Close()

				glog.Warningf("Your enterprise license has expired and enterprise features are " +
					"disabled. To continue using enterprise features, apply a valid license. " +
					"To receive a new license, contact us at https://dgraph.io/contact.")
				crashLearner()
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
	if _, err := st.zero.ApplyLicense(ctx, &pb.ApplyLicenseRequest{License: b}); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		x.SetStatus(w, x.ErrorInvalidRequest, err.Error())
		return
	}
	if _, err := w.Write([]byte(`{"code": "Success", "message": "License applied."}`)); err != nil {
		glog.Errorf("Unable to send http response. Err: %v\n", err)
	}
}

func (s *Server) applyLicenseFile(path string) {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		glog.Infof("Unable to apply license at %v due to error %v", path, err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if _, err = s.ApplyLicense(ctx, &pb.ApplyLicenseRequest{License: content}); err != nil {
		glog.Infof("Unable to apply license at %v due to error %v", path, err)
	}
}
