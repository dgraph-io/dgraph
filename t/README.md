# Dgraph Testing Framework

Dgraph employs a ~~complex~~ sophisticated testing framework that includes extensive test coverage. 
Due to the comprehensive nature of these tests, a complete test run can take several hours, depending
on your hardware. To manage this complex testing process efficiently, we've developed a custom test 
runner implemented in Go: [t.go](t.go). This specialized runner provides enhanced 
control and flexibility beyond what's available through the standard Go testing framework.

*Note: This testing framework was built with Linux in mind. Non-Linux testing *can* be achieved-see
the section below.*

## Requirements
Several external dependencies are required. You can check your system for the required dependencies
by running `make check`.

#### Go

Version 1.22.7 or higher.

#### Docker

The framework uses Docker extensively for integration testing. On non-Linux systems, tests will often
fail if Docker is not given enough resources, specifically memory. If you experience testing that 
seems to hang waiting for connections to other cluster members, it's probably a memory issue.

The test runner takes a concurrency flag (./t -j=N) which will attempt to create *N* number of Dgraph
clusters. If you are testing on a machine with limited resources, we advise you to not set this above *1*.

You can preserve the test Docker containers for failure analysis with the `--keep` flag.

#### gotestsum

The framework uses [gotestsum](https://github.com/gotestyourself/gotestsum#install) for collating output and other advanced functions.

#### protoc

On non-Linux system, protocol buffer tests are skipped. On Linux systems, instructions for installing
and configuring protoc can be found [here](https://github.com/protocolbuffers/protobuf). Or, `sudo apt update && sudo apt install -y protobuf-compiler`.

The script can be run like this:

```bash
$ go build . && ./t
# ./t --help to see the flags.
```

You can run a specific package or a specific test via this script. You can use the concurrency flag
to specify how many clusters to run.

---

This script runs many clusters of Dgraph. To make your tests work with this script, they can get the
address for any instance by passing in the name of the instance and the port:

`testutil.ContainerAddr("alpha2", 9080)`


Needed, gotestsum, protoc, docker (give Docker plenty of resources)


CD dgraph
make install
mv /Users/matthew/go/bin/dgraph /Users/matthew/go/bin/dgraph_osx
GOOS=linux make install
mv /Users/matthew/go/bin/linux_arm64/dgraph /Users/matthew/go/bin/dgraph
cd ../t
mkdir data
DGRAPH_BINARY=/Users/matthew/go/bin/dgraph_osx GOPATH=/Users/matthew/go DOCKER_HOST=unix:///Users/matthew/.docker/run/docker.sock make test args="--pkg=graphql/e2e/normal"

/t --dry for just the packages

./t --suite=core
