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
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/golang/glog"
	"github.com/mitchellh/panicwrap"
)

var (
	env string
	dsn string // API KEY to use
)

// Sentry API KEYs to use.
const (
	// dgraph-gh project (production/release builds).
	dsnProd = "https://58a035f0d85a4c1c80aee0a3e72f3899@o318308.ingest.sentry.io/1805390"
	// dgraph-devtest-playground project (dev builds).
	dsnDevtest = "https://84c2ad450005436fa27d97ef72b52425@o318308.ingest.sentry.io/5208688"
)

// InitSentry initializes the sentry machinery.
func InitSentry(ee bool) {
	env = "prod-"
	dsn = dsnProd
	if DevVersion() {
		dsn = dsnDevtest
		env = "dev-"
	}
	if ee {
		env += "enterprise"
	} else {
		env += "oss"
	}
	initSentry()
}

func initSentry() {
	if err := sentry.Init(sentry.ClientOptions{
		Dsn:              dsn,
		Debug:            true,
		AttachStacktrace: true,
		ServerName:       WorkerConfig.MyAddr,
		Environment:      env,
		Release:          Version(),
		BeforeSend: func(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
			// Modify the event here before sending it to sentry server.
			if len(event.Exception) == 0 {
				return event
			}
			ex := &event.Exception[0]
			// Filter out the stacktrace since it is of no use.
			ex.RawStacktrace = nil
			ex.Stacktrace = nil

			// Set exception type to the panic message.
			if strings.HasPrefix(event.Exception[0].Value, "panic") {
				indexofNewline := strings.IndexByte(event.Exception[0].Value, '\n')
				if indexofNewline != -1 {
					ex.Type = ex.Value[:indexofNewline]
				}
			}
			return event
		},
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
