/*
 * Copyright (C) 2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// Package y contains the code shared by the Go client and Dgraph server. It should have
// minimum dependencies in order to keep the client light.
package y

import (
	"errors"
)

var (
	ErrAborted  = errors.New("Transaction has been aborted. Please retry.")
	ErrConflict = errors.New("Conflicts with pending transaction. Please abort.")
)
