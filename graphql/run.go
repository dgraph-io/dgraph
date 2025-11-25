/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
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
