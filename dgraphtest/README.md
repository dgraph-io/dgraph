## Run Tests Against Dgraph Cloud

### Setup Env Variables

1. set `TEST_DGRAPH_CLOUD_CLUSTER_URL` to the URL that the dgo client should use
2. set `TEST_DGRAPH_CLOUD_CLUSTER_TOKEN` to one of API keys that you can generate on the `Settings`
   page on the cloud UI
3. `TEST_DGRAPH_CLOUD_ACL` to `false` if ACLs are disabled. By default, ACLs are assumed to be
   enabled.

### Schema Mode

The tests require the `Schema Mode` to be set to `Flexible` from the `Settings` page on the cloud
UI.

### Running Tests

```bash
go test -tags=cloud ./...
```
