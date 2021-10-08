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

import { evaluateScript } from './evaluate-script';
import { waitForDgraph, loadSchema, runQuery } from './test-utils'
import sleep from 'sleep-promise';

const integrationTest = process.env.INTEGRATION_TEST === "true" ? describe : describe.skip;

describe(evaluateScript, () => {
  const ns = "0"
  it("returns undefined if there was no event", async () => {
    const runScript = evaluateScript("", ns)
    expect(await runScript({type: "Query.unknown", args: {}, parents: null})).toBeUndefined()
  })

  it("returns the value if there is a resolver registered", async () => {
    const runScript = evaluateScript(`addGraphQLResolvers({
      "Query.fortyTwo": ({parent}) => 42
    })`, ns)
    expect(await runScript({ type: "Query.fortyTwo", args: {}, parents: null })).toEqual(42)
  })

  it("passes the args and parents over", async () => {
    const runScript = evaluateScript(`addMultiParentGraphQLResolvers({
      "User.fortyTwo": ({parents, args}) => parents.map(({n}) => n + args.foo)
    })`, ns)
    expect(await runScript({ type: "User.fortyTwo", args: {foo: 1}, parents: [{n: 41}] })).toEqual([42])
  })

  it("returns undefined if the number of parents doesn't match the number of return types", async () => {
    const runScript = evaluateScript(`addMultiParentGraphQLResolvers({
      "Query.fortyTwo": () => [41, 42]
    })`, ns)
    expect(await runScript({ type: "Query.fortyTwo", args: {}, parents: null })).toBeUndefined()
  })

  it("returns undefined somehow the script doesn't return an array", async () => {
    const runScript = evaluateScript(`addMultiParentGraphQLResolvers({
      "User.fortyTwo": () => ({})
    })`, ns)
    expect(await runScript({ type: "User.fortyTwo", args: {}, parents: [{n: 42}] })).toBeUndefined()
  })

  integrationTest("dgraph integration", () => {
    beforeAll(async () => {
      await waitForDgraph();
      await loadSchema(`type Todo { id: ID!, title: String! }`)
      await sleep(250)
    })

    it("works with dgraph graphql", async () => {
      const runScript = evaluateScript(`
        async function todoTitles({graphql}) {
          const results = await graphql('{ queryTodo { title } }')
          return results.data.queryTodo.map(t => t.title)
        }
        addGraphQLResolvers({ "Query.todoTitles": todoTitles })`, ns)
      const results = await runScript({ type: "Query.todoTitles", args: {}, parents: null });
      expect(new Set(results)).toEqual(new Set(["Kick Ass", "Chew Bubblegum"]))
    })

    it("works with dgraph dql", async () => {
      const runScript = evaluateScript(`
        async function todoTitles({dql}) {
          const results = await dql.query('{ queryTitles(func: type(Todo)){ Todo.title } }')
          return results.data.queryTitles.map(t => t["Todo.title"])
        }
        addGraphQLResolvers({ "Query.todoTitles": todoTitles })`, ns)
      const results = await runScript({ type: "Query.todoTitles", args: {}, parents: null });
      expect(new Set(results)).toEqual(new Set(["Kick Ass", "Chew Bubblegum"]))
    })
  })
})
