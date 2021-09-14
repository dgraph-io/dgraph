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

import { EventTarget } from 'event-target-shim';
import vm from 'vm';
import { GraphQLEvent, GraphQLEventWithParent, GraphQLEventFields, ResolverResponse, AuthHeaderField, WebHookGraphQLEvent } from '@dgraph-lambda/lambda-types'

import fetch, { RequestInfo, RequestInit, Request, Response, Headers } from "node-fetch";
import { URL } from "url";
import isIp from "is-ip";
import atob from "atob";
import btoa from "btoa";
import { TextDecoder, TextEncoder } from "util";
import { Crypto } from "node-webcrypto-ossl";
import { graphql, dql } from './dgraph';

function getParents(e: GraphQLEventFields): (Record<string,any>|null)[] {
  return e.parents || [null]
}

class GraphQLResolverEventTarget extends EventTarget {
  console: Console;
  constructor(c: Console) {
    super();
    this.console = c
  }
  addMultiParentGraphQLResolvers(resolvers: {[key: string]: (e: GraphQLEvent) => ResolverResponse}) {
    for (const [name, resolver] of Object.entries(resolvers)) {
      this.addEventListener(name, e => {
        try {
          const event = e as unknown as GraphQLEvent;
          event.respondWith(resolver(event))
        } catch(e: any) {
          this.console.error(e.toString() + JSON.stringify(e.stack))
          return
        }
      })
    }
  }

  addGraphQLResolvers(resolvers: { [key: string]: (e: GraphQLEventWithParent) => (any | Promise<any>) }) {
    for (const [name, resolver] of Object.entries(resolvers)) {
      this.addEventListener(name, e => {
        try {
          const event = e as unknown as GraphQLEvent;
          event.respondWith(getParents(event).map(parent => resolver({...event, parent})))
        } catch(e: any) {
          this.console.error(e.toString() + JSON.stringify(e.stack))
          return
        }
      })
    }
  }

  addWebHookResolvers(resolvers: { [key: string]: (e: WebHookGraphQLEvent) => (any | Promise<any>) }) {
    for (const [name, resolver] of Object.entries(resolvers)) {
      this.addEventListener(name, e => {
        try {
          const event = e as unknown as WebHookGraphQLEvent;
          event.respondWith(resolver(event))
        } catch(e: any) {
          this.console.error(e.toString() + JSON.stringify(e.stack))
          return
        }
      })
    }
  }
}

function newConsole(prefix: string) {
  // Override the console object to append prefix to the logs.
  const appendPrefix = function(fn: (message?: any, ...optionalParams: any[]) => void, prefix: string) {
    return function() {
      fn.apply(console, [prefix + Array.from(arguments).map(arg => JSON.stringify(arg)).join(" ")])
    }
  }
  const _console = Object.assign({}, console)
  _console.debug = appendPrefix(console.debug, prefix)
  _console.error = appendPrefix(console.error, prefix)
  _console.info = appendPrefix(console.info, prefix)
  _console.log = appendPrefix(console.log, prefix)
  _console.warn = appendPrefix(console.warn, prefix)
  return _console
}

const fetchTimeout = 10000 // 10s
function fetchWithMiddleWare(url: RequestInfo, init?: RequestInit): Promise<Response> {
  // Override the default fetch to blacklist certain IPs.
  try {
    const u = new URL(url.toString())
    if (isIp(u.hostname) || u.hostname == "localhost") {
      return new Promise((_resolve, reject) => {
        reject("Cannot send request to IP: " + url.toString()
        + ". Please use domain names instead.")
        return
      })
    }
  } catch(error) {
    return new Promise((_resolve, reject) => {
      reject(error)
      return
    })
  }
  // Add a timeout of 10s.
  if(init === undefined) {
    init = {}
  }
  if(init.timeout === undefined || init.timeout > fetchTimeout) {
    init.timeout = fetchTimeout
  }
  return fetch(url, init)
}

function newContext(eventTarget: GraphQLResolverEventTarget, c: Console) {
  return vm.createContext({
    // From fetch
    fetch:fetchWithMiddleWare,
    Request,
    Response,
    Headers,

    // URL Standards
    URL,
    URLSearchParams,

    // bas64
    atob:atob.bind({}),
    btoa:btoa.bind({}),

    // Crypto
    crypto: new Crypto(),
    TextDecoder,
    TextEncoder,

    // Debugging
    console:c,

    // EventTarget
    self: eventTarget,
    addEventListener: eventTarget.addEventListener.bind(eventTarget),
    removeEventListener: eventTarget.removeEventListener.bind(eventTarget),
    addMultiParentGraphQLResolvers: eventTarget.addMultiParentGraphQLResolvers.bind(eventTarget),
    addGraphQLResolvers: eventTarget.addGraphQLResolvers.bind(eventTarget),
    addWebHookResolvers: eventTarget.addWebHookResolvers.bind(eventTarget),
  });
}

export function evaluateScript(source: string, prefix: string) {
  const script = new vm.Script(source)
  const _console = newConsole(prefix);
  const target = new GraphQLResolverEventTarget(_console);
  const context = newContext(target, _console)
  // Using the timeout or breakOnSigint options will result in new event loops and corresponding
  // threads being started, which have a non-zero performance overhead.
  // Ref: https://nodejs.org/api/vm.html#vm_script_runincontext_contextifiedobject_options
  // It should not take more than a second to add the resolvers. Add timeout of 1 second.
  script.runInContext(context, {timeout: 1000});

  return async function(e: GraphQLEventFields): Promise<any | undefined> {
    let retPromise: ResolverResponse | undefined = undefined;
    const event = {
      ...e,
      respondWith: (x: ResolverResponse) => { retPromise = x },
      graphql: (query: string, variables: Record<string, any>, ah?: AuthHeaderField, token?: string) => graphql(query, variables, ah || e.authHeader, token || e.accessToken),
      dql: {
        query: (query: string, variables: Record<string, any> = {}, token?:string) => dql.query(query, variables, token || e.accessToken),
        mutate: (mutate: string | Object, token?: string) => dql.mutate(mutate, token || e.accessToken),
      }
    }
    if (e.type === '$webhook' && e.event) {
      event.type = `${e.event?.__typename}.${e.event?.operation}` 
    }
    target.dispatchEvent(event)

    if(retPromise === undefined) {
      return undefined
    }

    const resolvedArray = await (retPromise as ResolverResponse);
    if(!Array.isArray(resolvedArray) || resolvedArray.length !== getParents(e).length) {
      process.env.NODE_ENV != "test" && e.type !== '$webhook' && console.error(`Value returned from ${e.type} was not an array or of incorrect length`)
      return undefined
    }

    const response = await Promise.all(resolvedArray);
    return e.parents === null ? response[0] : response;
  }
}
