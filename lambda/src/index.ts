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