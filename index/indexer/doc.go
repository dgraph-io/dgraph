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

// Package indexer is an interface to indexing solutions such as Bleve.
// There are 3 operations: Insert, Delete, Query. Let P=predicate, K=key, V=value.
// Insert(P, K, V):
//   (1) If K already exists with a different value, we expect Indexer to remove
//       the old value from the index.
//   (2) Otherwise, we insert (K, V) into the index.
// Delete(P, K):
//   (1) If K exists, we do the deletion.
//   (2) If K does not exist, nothing happens. No error is returned.
// Query(P, V): Return a sorted list of keys.
package indexer
