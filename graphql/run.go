/*
 * Copyright 2022 Dgraph Labs, Inc. and Contributors
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

// Package graphql is a http server for GraphQL on Dgraph
//
// GraphQL spec:
// https://graphql.github.io/graphql-spec/June2018
//
// GraphQL servers should serve both GET and POST
// https://graphql.org/learn/serving-over-http/
//
// GET should be like
// http://myapi/graphql?query={me{name}}
//
// POST should have a json content body like
//
//	{
//	  "query": "...",
//	  "operationName": "...",
//	  "variables": { "myVariable": "someValue", ... }
//	}
//
// GraphQL servers should return 200 (even on errors),
// and result body should be json:
//
//	{
//	  "data": { "query_name" : { ... } },
//	  "errors": [ { "message" : ..., ...} ... ]
//	}
//
// Key points about the response
// (https://graphql.github.io/graphql-spec/June2018/#sec-Response)
//
//   - If an error was encountered before execution begins,
//     the data entry should not be present in the result.
//
//   - If an error was encountered during the execution that
//     prevented a valid response, the data entry in the response should be null.
//
// - If there's errors and data, both are returned
//
//   - If no errors were encountered during the requested operation,
//     the errors entry should not be present in the result.
//
//   - There's rules around how errors work when there's ! fields in the schema
//     https://graphql.github.io/graphql-spec/June2018/#sec-Errors-and-Non-Nullability
//
// - The "message" in an error is required, the rest is up to the implementation
//
// - The "data" works just like a Dgraph query
//
// - "extensions" is allowed and can be anything
package graphql
