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

// Package icuembed contains ICU common lib. It is for embedding into our binary
// so that the user does not have to install ICU.
package icuembed

// #cgo CPPFLAGS: -O2 -DU_COMMON_IMPLEMENTATION  -DU_DISABLE_RENAMING=1 -Wno-deprecated-declarations
// #cgo LDFLAGS: -ldl
import "C"
