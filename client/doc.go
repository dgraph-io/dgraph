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
mutations can be run from the client.

- Request based interaction mode where the user program builds requests
and receives responses immediately after running, and

Request Mode: User programs create a NewDgraphClient, create a request
with req := client.Req{} and then add edges, deletion, schema and queries
to the request with SetObject/DeleteObject, AddSchema/SetSchema and SetQuery/SetQueryWithVariables.
Once the request is built, it is run with Run. This is the mode that would be suitable for most
real-time applications. Below is an example on how to use SetObject to add some data to Dgraph.

For more details checkout https://docs.dgraph.io/clients/#go.
*/
package client
