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

import fetch from 'node-fetch';
import { GraphQLResponse, AuthHeaderField } from '@dgraph-lambda/lambda-types';

export async function graphql(query: string, variables: Record<string, any> = {}, authHeader?: AuthHeaderField, accessToken?: string): Promise<GraphQLResponse> {
  const headers: Record<string, string> = { "Content-Type": "application/json" };
  if (authHeader && authHeader.key && authHeader.value) {
    headers[authHeader.key] = authHeader.value;
  }
  headers['X-Dgraph-AccessToken'] = accessToken || ""
  const response = await fetch(`${process.env.DGRAPH_URL}/graphql`, {
    method: "POST",
    headers,
    body: JSON.stringify({ query, variables })
  })
  if (response.status !== 200) {
    throw new Error("Failed to execute GraphQL Query")
  }
  return response.json();
}

async function dqlQuery(query: string, variables: Record<string, any> = {}, accessToken?:string): Promise<GraphQLResponse> {
  const response = await fetch(`${process.env.DGRAPH_URL}/query`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "X-Dgraph-AccessToken": accessToken || "",
    },
    body: JSON.stringify({ query, variables })
  })
  if (response.status !== 200) {
    throw new Error("Failed to execute DQL Query")
  }
  return response.json();
}

async function dqlMutate(mutate: string | Object, accessToken?: string): Promise<GraphQLResponse> {
  const response = await fetch(`${process.env.DGRAPH_URL}/mutate?commitNow=true`, {
    method: "POST",
    headers: {
      "Content-Type": typeof mutate === 'string' ? "application/rdf" : "application/json",
      "X-Dgraph-AccessToken": accessToken || "",
    },
    body: typeof mutate === 'string' ? mutate : JSON.stringify(mutate)
  })
  if (response.status !== 200) {
    throw new Error("Failed to execute DQL Mutate")
  }
  return response.json();
}

export const dql = {
  query: dqlQuery,
  mutate: dqlMutate,
}
