/*
 * Copyright 2021 Dgraph Labs, Inc. All rights reserved.
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
 */

import { EventTarget } from 'event-target-shim';
import vm from 'vm';
import { GraphQLEvent, GraphQLEventWithParent, GraphQLEventFields, ResolverResponse, AuthHeaderField, WebHookGraphQLEvent } from '@slash-graphql/lambda-types'

import fetch, { Request, Response, Headers } from "node-fetch";
import { URL } from "url";
import atob from "atob";
import btoa from "btoa";
import { TextDecoder, TextEncoder } from "util";
import { Crypto } from "node-webcrypto-ossl";
import { graphql, dql } from './dgraph';

function getParents(e: GraphQLEventFields): (Record<string,any>|null)[] {
  return e.parents || [null]
}

class GraphQLResolverEventTarget extends EventTarget {
  addMultiParentGraphQLResolvers(resolvers: {[key: string]: (e: GraphQLEvent) => ResolverResponse}) {
    for (const [name, resolver] of Object.entries(resolvers)) {
      this.addEventListener(name, e => {
        const event = e as unknown as GraphQLEvent;
        event.respondWith(resolver(event))
      })
    }
  }

  addGraphQLResolvers(resolvers: { [key: string]: (e: GraphQLEventWithParent) => (any | Promise<any>) }) {
    for (const [name, resolver] of Object.entries(resolvers)) {
      this.addEventListener(name, e => {
        const event = e as unknown as GraphQLEvent;
        event.respondWith(getParents(event).map(parent => resolver({...event, parent})))
      })
    }
  }

  addWebHookResolvers(resolvers: { [key: string]: (e: WebHookGraphQLEvent) => (any | Promise<any>) }) {
    for (const [name, resolver] of Object.entries(resolvers)) {
      this.addEventListener(name, e => {
        const event = e as unknown as WebHookGraphQLEvent;
        event.respondWith(resolver(event))
      })
    }
  }
}

function newContext(eventTarget: GraphQLResolverEventTarget, logger: any) {
  function appendLogs(){
    return function() {
      var args = Array.from(arguments);
      logger.logs = [logger.logs, ...args].join("\n")
    }
  }

  // Override the console object to append to logger.logs.
  const _console = Object.assign({}, console)
  _console.debug = appendLogs()
  _console.error = appendLogs()
  _console.info = appendLogs()
  _console.log = appendLogs()
  _console.warn = appendLogs()

  return vm.createContext({
    // From fetch
    fetch:fetch.bind({}),
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
    console:_console,

    // Async
    setTimeout,
    setInterval,
    clearTimeout,
    clearInterval,

    // EventTarget
    self: eventTarget,
    addEventListener: eventTarget.addEventListener.bind(eventTarget),
    removeEventListener: eventTarget.removeEventListener.bind(eventTarget),
    addMultiParentGraphQLResolvers: eventTarget.addMultiParentGraphQLResolvers.bind(eventTarget),
    addGraphQLResolvers: eventTarget.addGraphQLResolvers.bind(eventTarget),
    addWebHookResolvers: eventTarget.addWebHookResolvers.bind(eventTarget),
  });
}


var scripts = new Map();

export function evaluateScript(source: string, logger: any) {
  if(!scripts.has(source)){
    scripts.set(source, new vm.Script(source))
  }
  const script = scripts.get(source)
  const target = new GraphQLResolverEventTarget();
  const context = newContext(target, logger)
  // Using the timeout or breakOnSigint options will result in new event loops and corresponding
  // threads being started, which have a non-zero performance overhead.
  // Ref: https://nodejs.org/api/vm.html#vm_script_runincontext_contextifiedobject_options
  script.runInContext(context, {timeout:10000}); // timeout after 10 seconds

  return async function(e: GraphQLEventFields): Promise<any | undefined> {
    let retPromise: ResolverResponse | undefined = undefined;
    const event = {
      ...e,
      respondWith: (x: ResolverResponse) => { retPromise = x },
      graphql: (query: string, variables: Record<string, any>, ah?: AuthHeaderField) => graphql(query, variables, ah || e.authHeader),
      dql,
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
