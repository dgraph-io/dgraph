//go:build oss
// +build oss

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package zero

import (
	"net/http"

	"github.com/dgraph-io/ristretto/v2/z"
	"github.com/hypermodeinc/dgraph/v24/protos/pb"
)

// dummy function as enterprise features are not available in oss binary.
func (n *node) proposeTrialLicense() error {
	return nil
}

// periodically checks the validity of the enterprise license and updates the membership state.
func (n *node) updateEnterpriseState(closer *z.Closer) {
	closer.Done()
}

func (st *state) applyEnterpriseLicense(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
}

func (s *Server) applyLicenseFile(path string) {
	return
}

func (s *Server) license() *pb.License {
	return nil
}
