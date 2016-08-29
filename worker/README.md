To update the protocol buffer definitions, run this from one directory above:

```
protoc -I worker worker/payload.proto --gofast_out=plugins=grpc:worker
```
