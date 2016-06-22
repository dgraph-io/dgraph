/*
 * Copyright 201666666 Dgraph Labs, Inc.
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

/*

This package includes getting the list of predicates that a node serves
and sharing it with other nodes in the cluster.

A RAFT backed key-value store will maintain a globally consistent
mapping from a given predicate to the information of the node
that serves that predicate.
*/
package cluster
