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
