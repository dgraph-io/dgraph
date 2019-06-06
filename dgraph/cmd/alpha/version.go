/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors
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

package alpha

import (
	"encoding/json"
	"net/http"

	"github.com/dgraph-io/dgraph/x"
)

func versionHandler(w http.ResponseWriter, r *http.Request) {
	if err := x.HealthCheck(); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	info := struct {
		AlphaVersion string `json:"alpha_version"`
	}{
		AlphaVersion: x.Version(),
	}
	data, _ := json.Marshal(info)

	x.AddCorsHeaders(w)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
