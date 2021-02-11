/*
 * Copyright 2015-2020 Dgraph Labs, Inc. and Contributors
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

type ExportedGQLSchema struct {
	Namespace uint64
	Schema    string
}

// SensitiveByteSlice implements the Stringer interface to redact its contents.
// Use this type for sensitive info such as keys, passwords, or secrets so it doesn't leak
// as output such as logs.
type SensitiveByteSlice []byte

func (SensitiveByteSlice) String() string {
	return "****"
}
