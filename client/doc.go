/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
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


/* 
Package client is used to interact with a Dgraph server.  Queries and
mutations can be run from the client.  There are essentially two modes
of client interaction:

- Request based interaction mode where the user program builds requests 
and receives responses immediately after running, and

- Batch mode where clients submit many requests and let the client package
batch those requests to the server.

Request Mode: User programs create a NewDgraphClient, create a request
with req := client.Req{} and then add edges, deletion, schema and
queries to the request with Set, Delete, AddSchema/AddSchemaFromString and
SetQuery/SetQueryWithVariables.  Once the request is built, it is run with
Run.  

Batch Mode:  On creating a new client with NewDgraphClient users submit
BatchMutationOptions specifying the size of batches and number of concurrent 
batches.  Edges are added to the batch with BatchSet; deletions are added
with BatchDelete; and schema mutations with AddSchema.

Submitted mutations are nondeterministically added to batches and there are
no guarantees about which batch a mutation will be scheduled for (e.g. two
successive calls to BatchSet won't guarantee the edges to be in the same batch).

Finishing and interaction with BatchFlush flushes all buffers and ends the
client interaction.

For more details checkout https://docs.dgraph.io/clients/#go.
*/
package client
