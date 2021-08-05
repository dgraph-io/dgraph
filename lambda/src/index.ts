/*
 * Copyright 2021 Dgraph Labs, Inc. All rights reserved.
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
 */

import cluster from "cluster";
import { buildApp } from "./buildApp"

async function startServer() {
  const app = buildApp()
  const port = process.env.PORT || "8686";
  const server = app.listen(port, () =>
    console.log("Server Listening on port " + port + "!")
  );
  cluster.on("disconnect", () => server.close());
  process.on("SIGINT", () => {
    server.close();
    process.exit(0);
  });
}

startServer();