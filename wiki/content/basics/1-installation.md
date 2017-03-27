+++
title="Step 1: Installation"
section = "basics"
categories = ["basics"]
weight=1
slug="installation"

[menu.main]
    url = "installation"
    parent = "basics"

+++

### System Installation

You could simply install the binaries with
```
$ curl https://get.dgraph.io -sSf | bash
```

That script would automatically install Dgraph for you. Once done, you can jump straight to step 2.

**Alternative:** To mitigate potential security risks, you could instead do this:
```
$ curl https://get.dgraph.io > /tmp/get.sh
$ vim /tmp/get.sh  # Inspect the script
$ sh /tmp/get.sh   # Execute the script
```

### Docker Image Installation

You may pull our Docker images [from here](https://hub.docker.com/r/dgraph/dgraph/). From terminal, just type:
```
docker pull dgraph/dgraph
```
