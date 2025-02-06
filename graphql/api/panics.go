/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
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
func PanicHandler(fn func(error), query string) {
	if err := recover(); err != nil {
		// Log the panic along with query which caused it.
		glog.Errorf("panic: %s.\n query: %s\n trace: %s", err, query, string(debug.Stack()))

		fn(errors.Errorf("Internal Server Error - a panic was trapped.  " +
			"This indicates a bug in the GraphQL server.  A stack trace was logged.  " +
			"Please let us know by filing an issue with the stack trace."))
	}
}
