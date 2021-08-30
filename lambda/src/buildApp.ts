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
import timeout from "connect-timeout";
import vm from 'vm';
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

export function buildApp() {
    const app = express();
    app.use(timeout('10s'))
    app.use(express.json({limit: '32mb'}))
    app.post("/graphql-worker", async (req, res, next) => {
        const logger = {logs: ""}
        try {
          const source = base64Decode(req.body.source) || req.body.source
          const runner = evaluateScript(source, logger)
          await vm.runInNewContext(
            `runner(bodyToEvent(req.body)).then(result => {
              if(result === undefined && req.body.resolver !== '$webhook') {
                  res.status(400)
              }
              res.json({res: result, logs: logger.logs})
            })
            `,
            {runner, bodyToEvent, req, res, logger},
            {timeout:10000}) // timeout after 10 seconds
        } catch(err) {
          res.status(500)
          res.json({logs: logger.logs + err.toString()})
        }
    })
    return app;
}
