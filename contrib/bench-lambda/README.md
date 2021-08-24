## Steps to test:

1. Start dgraph with lambda servers.
```
dgraph zero
dgraph alpha --lambda num=2
```

2. Run `load-data.sh` script
3. Now run the benchmark spitting out the qps.
```
go run main.go --lambda
```

