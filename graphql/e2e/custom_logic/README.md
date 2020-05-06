The test file should be run after bringing up the docker containers via docker-compose.
Since the tests rely on a mock server, which is implemented via cmd/main.go, run the following
command.

```
docker-compose up --build
```

This command would cause a force rebuild of the docker image for the mock server anytime a change is
made to the cmd/main.go file.
