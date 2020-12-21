+++
date = "2017-03-20T22:25:17+11:00"
title = "Data compression on Disk"
weight = 17
[menu.main]
    parent = "deploy"
+++

Dgraph Alpha lets you configure the compression of data on disk using the
`--badger.compression` option. You can choose between the
[Snappy](https://github.com/golang/snappy) and
[Zstandard](https://github.com/facebook/zstd) compression algorithms, or choose
not to compress data on disk.

{{% notice "note" %}}This option replaces the  `--badger.compression_level`
option used in earlier Dgraph versions. {{% /notice %}}

The following disk compression settings are available:

| Setting    | Notes                                                                |
|------------|----------------------------------------------------------------------|
|`none`      | Data on disk will not be compressed.                                 |
|`zstd:level`| Use Zstandard compression, with a compression level specified (1-3). |
|`snappy`    | Use Snappy compression (this is the default value).                  |

For example, you could choose to use Zstandard compression with the highest
compression level using the following command:

```sh
dgraph alpha --badger.compression=zstd:3
```

This compression setting (Zstandard, level 3) is more CPU-intensive than other
options, but offers the highest compression ratio. To change back to the default
compression setting, use the following command:


```sh
dgraph alpha --badger.compression=snappy
```

Using this compression setting (Snappy) provides a good compromise between the
need for a high compression ratio and efficient CPU usage.
