/*
 * Copyright 2021 Dgraph Labs, Inc. All rights reserved.
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
 */

import express from "express";
import atob from "atob";
import btoa from "btoa";
import { evaluateScript } from './evaluate-script'
import { GraphQLEventFields } from '@slash-graphql/lambda-types'

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
    app.use(express.json({limit: '32mb'}))
    app.post("/graphql-worker", async (req, res, next) => {
        const logger = {logs: ""}
        try {
          const source = base64Decode(req.body.source) || req.body.source
          const runner = evaluateScript(source, logger)
          const result = await runner(bodyToEvent(req.body));
          if(result === undefined && req.body.resolver !== '$webhook') {
              res.status(400)
          }
          res.json({res:result, logs: logger.logs})
        } catch(err) {
          res.json({logs: logger.logs, error: err.toString()})
          res.status(500)
        }
    })
    return app;
}
