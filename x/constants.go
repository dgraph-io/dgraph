/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
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

// Constants for sizes in bytes. Using these makes code more readable than using bit
// shifting (1 << 20), or exponential notation (1e6), or large numbers (1000000).
const (
	KiB int = 1024
	MiB     = KiB * 1024
	GiB     = MiB * 1024
	TiB     = GiB * 1024
)

// Constants for generic counts. Useful for making large numbers more readable.
const (
	Thousand     = 1000
	Million      = Thousand * 1000
	Billion      = Million * 1000
	Trillion     = Billion * 1000
)
