/*
 * Copyright 2016 Dgraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// Package tok is a wrapper around ICU boundary iterating functions.
package tok

import "C"

// Note that currently, in a lot of files, we have the line "+build tok" to
// enable the file only when we see the build tag "tok". The reason is that we
// are not asking the user to install ICU yet, until we try harder to embed it.
// To run the build, do "go build -tags tok ." or to run the test in this folder,
// to "go test -v -tags tok ."
//
// After we sort out the above, we will in a separate PR, use this package in
// index.go, as well as add a new filter function to use this index. We are still
// waiting for the filter logic in query package to be checked in.
