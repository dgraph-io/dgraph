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

package x

import (
	"errors"
	"os"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/golang/glog"
	"github.com/mitchellh/panicwrap"
)

var env string

// InitSentry initializes the sentry machinery.
func InitSentry(ee bool) {
	if ee {
		env = "enterprise"
	} else {
		env = "oss"
	}
	initSentry()
}

func initSentry() {
	if err := sentry.Init(sentry.ClientOptions{
		Dsn:              "https://58a035f0d85a4c1c80aee0a3e72f3899@sentry.io/1805390",
		Debug:            true,
		AttachStacktrace: true,
		ServerName:       WorkerConfig.MyAddr,
		Environment:      env,
		Release:          Version(),
	}); err != nil {
		glog.Fatalf("Sentry init failed: %v", err)
	}
}

// FlushSentry flushes the buffered events/errors.
func FlushSentry() {
	sentry.Flush(time.Second * 2)
}

// ConfigureSentryScope configures the scope on the global hub of Sentry.
func ConfigureSentryScope(subcmd string) {
	sentry.ConfigureScope(func(scope *sentry.Scope) {
		scope.SetTag("dgraph", subcmd)
		scope.SetLevel(sentry.LevelFatal)
	})
}

// Panic sends the error report to Sentry and then panics.
func Panic(err error) {
	if err != nil {
		CaptureSentryException(err)
		panic(err)
	}
}

// CaptureSentryException sends the error report to Sentry.
func CaptureSentryException(err error) {
	if err != nil {
		sentry.CaptureException(err)
	}
}

// PanicHandler is the callback function when a panic happens. It does not recover and is
// only used to log panics (in our case send an event to sentry).
func PanicHandler(out string) {
	// Output contains the full output (including stack traces) of the panic.
	sentry.CaptureException(errors.New(out))
	FlushSentry() // Need to flush asap. Don't defer here.

	os.Exit(1)
}

// WrapPanics is a wrapper on panics. We use it to send sentry events about panics
// and crash right after.
func WrapPanics() {
	exitStatus, err := panicwrap.BasicWrap(PanicHandler)
	if err != nil {
		panic(err)
	}
	// If exitStatus >= 0, then we're the parent process and the panicwrap
	// re-executed ourselves and completed. Just exit with the proper status.
	if exitStatus >= 0 {
		os.Exit(exitStatus)
	}
}
