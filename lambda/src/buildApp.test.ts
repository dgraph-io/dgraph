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

import { buildApp } from "./buildApp";
import supertest from 'supertest'

describe(buildApp, () => {
  const app = buildApp()

  it("calls the appropriate function, passing the resolver, parent and args", async () => {
    const source = `addMultiParentGraphQLResolvers({
      "Query.fortyTwo": ({parents, args}) => parents.map(({n}) => n + args.foo)
    })`
    const response = await supertest(app)
      .post('/graphql-worker')
      .send({ source: source, resolver: "Query.fortyTwo", parents: [{ n: 41 }], args: {foo: 1} })
      .set('Accept', 'application/json')
      .expect('Content-Type', /json/)
      .expect(200);
    expect(response.body).toEqual({"logs": "", "res": [42]});
  })

  it("returns a single item if the parents is null", async () => {
    const source = `addGraphQLResolvers({
      "Query.fortyTwo": () => 42
    })`
    const response = await supertest(app)
      .post('/graphql-worker')
      .send(
        { source: source, resolver: "Query.fortyTwo" },
      )
      .set('Accept', 'application/json')
      .expect('Content-Type', /json/)
      .expect(200);
    expect(response.body).toEqual({"logs": "", "res": 42});
  })

  it("returns a 400 if the resolver is not registered or invalid", async () => {
    const response = await supertest(app)
      .post('/graphql-worker')
      .send(
        { source: ``, resolver: "Query.notFound" },
      )
      .set('Accept', 'application/json')
      .expect('Content-Type', /json/)
      .expect(400);
    expect(response.body).toEqual({"logs": ""});
  })

  it("gets the auth header as a key", async () => {
    const source = `addGraphQLResolvers({
      "Query.authHeader": ({authHeader}) => authHeader.key + authHeader.value
    })`
    const response = await supertest(app)
      .post('/graphql-worker')
      .send({
        source: source, 
        resolver: "Query.authHeader", 
        parents: [{ n: 41 }], 
        authHeader: {key: "foo", value: "bar"} 
      })
      .set('Accept', 'application/json')
      .expect('Content-Type', /json/)
      .expect(200);
    expect(response.body).toEqual({"logs": "", "res": ["foobar"]});
  })
})
