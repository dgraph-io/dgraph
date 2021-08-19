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
          res.json({logs: logger.logs, error: err.toString()})
          res.status(500)
        }
    })
    return app;
}
