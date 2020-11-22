+++
date = "2017-03-20T22:25:17+11:00"
title = "Data compression on disk"
weight = 17
[menu.main]
    parent = "deploy"
+++

Alpha exposes the option `--badger.compression_level` to configure the compression
level for data on disk using Zstd compression. The option can be set as

```sh
dgraph alpha --badger.compression_level=xxx
```

A higher compression level is more CPU intensive but offers a better compression
ratio. The default level is 3.

This option is available in v20.03.1 and later.