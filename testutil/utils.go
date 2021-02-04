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

func DefaultNamespaceAttr(attr string) string {
	return x.NamespaceAttr(x.DefaultNamespace, attr)
}

func DefaultSchemaKey(attr string) []byte {
	attr = x.NamespaceAttr(x.DefaultNamespace, attr)
	return x.SchemaKey(attr)
}

func DefaultTypeKey(attr string) []byte {
	attr = x.NamespaceAttr(x.DefaultNamespace, attr)
	return x.TypeKey(attr)
}

func DefaultDataKey(attr string, uid uint64) []byte {
	attr = x.NamespaceAttr(x.DefaultNamespace, attr)
	return x.DataKey(attr, uid)
}

func DefaultReverseKey(attr string, uid uint64) []byte {
	attr = x.NamespaceAttr(x.DefaultNamespace, attr)
	return x.ReverseKey(attr, uid)
}

func DefaultIndexKey(attr, term string) []byte {
	attr = x.NamespaceAttr(x.DefaultNamespace, attr)
	return x.IndexKey(attr, term)
}

func DefaultCountKey(attr string, count uint32, reverse bool) []byte {
	attr = x.NamespaceAttr(x.DefaultNamespace, attr)
	return x.CountKey(attr, count, reverse)
}
