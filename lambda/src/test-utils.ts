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
import sleep from 'sleep-promise';

export async function waitForDgraph() {
  const startTime = new Date().getTime();
  while(true) {
    try {
      const response = await fetch(`${process.env.DGRAPH_URL}/probe/graphql`)
      if(response.status === 200) {
        return
      }
    } catch(e) { }
    await sleep(100);
    if(new Date().getTime() - startTime > 20000) {
      throw new Error("Failed while waiting for dgraph to come up")
    }
  }
}

export async function loadSchema(schema: string) {
  const response = await fetch(`${process.env.DGRAPH_URL}/admin/schema`, {
    method: "POST",
    headers: { "Content-Type": "application/graphql" },
    body: schema
  })
  if(response.status !== 200) {
    throw new Error("Could Not Load Schema")
  }
}

export async function runQuery(query: string) {
  const response = await fetch(`${process.env.DGRAPH_URL}/graphql`, {
    method: "POST",
    headers: { "Content-Type": "application/graphql" },
    body: query
  })
  if (response.status !== 200) {
    throw new Error("Could Not Fire GraphQL Query")
  }
}
