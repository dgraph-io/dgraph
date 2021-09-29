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

import express from "express";
import atob from "atob";
import btoa from "btoa";
import { evaluateScript } from './evaluate-script'
import { GraphQLEventFields } from '@dgraph-lambda/lambda-types'

function bodyToEvent(b: any): GraphQLEventFields {
  return {
    type: b.resolver,
    parents: b.parents || null,
    args: b.args || {},
    authHeader: b.authHeader,
    accessToken: b['X-Dgraph-AccessToken'],
    event: b.event || {},
    info: b.info || null,
  }
}

function base64Decode(str: string) {
  try {
    const original = str.trim();
    const decoded = atob(original);
    return btoa(decoded) === original ? decoded : "";
  } catch (err) {
    console.error(err);
    return "";
  }
}

var scripts = new Map()

export function buildApp() {
    const app = express();
    app.use(express.json({limit: '32mb'}))
    app.get("/health", (_req, res) => {
      res.status(200)
      res.json("HEALTHY")
    })
    app.post("/graphql-worker", async (req, res, next) => {
        const ns = req.body.namespace || 0
        const logPrefix = `[LAMBDA-${ns}] `
        try {
          const source = base64Decode(req.body.source) || req.body.source
          const key = ns + source
          if (!scripts.has(key)) {
            scripts.set(key, evaluateScript(source, logPrefix))
          }
          const runner = scripts.get(key)
          const result = await runner(bodyToEvent(req.body));
          if(result === undefined && req.body.resolver !== '$webhook') {
              res.status(400)
          }
          res.json(result)
        } catch(e: any) {
          console.error(logPrefix + e.toString() + JSON.stringify(e.stack))
          next(e)
        }
    })
    return app;
}
