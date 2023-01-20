//go:build oss
// +build oss

/*
 * Copyright 2022 Dgraph Labs, Inc. and Contributors
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

package zero

import (
	"net/http"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/ristretto/z"
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
