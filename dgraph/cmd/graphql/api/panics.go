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

package api

import (
	"runtime/debug"

	"github.com/golang/glog"
	"github.com/pkg/errors"
)

// PanicHandler catches panics to make sure that we recover from panics during
// GraphQL request execution and return an appropriate error.
//
// If PanicHandler recovers from a panic, it logs a stack trace, creates an error
// and applies fn to the error.
func PanicHandler(reqestID string, fn func(error)) {
	if err := recover(); err != nil {
		glog.Errorf("[%s] panic: %s.\n trace: %s", reqestID, err, string(debug.Stack()))

		fn(errors.Errorf("[%s] Internal Server Error - a panic was trapped.  "+
			"This indicates a bug in the GraphQL server.  A stack trace was logged.  "+
			"Please let us know : https://github.com/dgraph-io/dgraph/issues.",
			reqestID))
	}
}
