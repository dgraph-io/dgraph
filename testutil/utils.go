/*
 * Copyright 2021 Dgraph Labs, Inc. and Contributors
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

package testutil

import "github.com/dgraph-io/dgraph/x"

func GalaxySchemaKey(attr string) []byte {
	attr = x.GalaxyAttr(attr)
	return x.SchemaKey(attr)
}

func GalaxyTypeKey(attr string) []byte {
	attr = x.GalaxyAttr(attr)
	return x.TypeKey(attr)
}

func GalaxyDataKey(attr string, uid uint64) []byte {
	attr = x.GalaxyAttr(attr)
	return x.DataKey(attr, uid)
}

func GalaxyReverseKey(attr string, uid uint64) []byte {
	attr = x.GalaxyAttr(attr)
	return x.ReverseKey(attr, uid)
}

func GalaxyIndexKey(attr, term string) []byte {
	attr = x.GalaxyAttr(attr)
	return x.IndexKey(attr, term)
}

func GalaxyCountKey(attr string, count uint32, reverse bool) []byte {
	attr = x.GalaxyAttr(attr)
	return x.CountKey(attr, count, reverse)
}
