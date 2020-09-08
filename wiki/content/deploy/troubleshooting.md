+++
date = "2017-03-20T22:25:17+11:00"
title = "Troubleshooting"
[menu.main]
    parent = "deploy"
    weight = 19
+++

Here are some problems that you may encounter and some solutions to try.

### Running OOM (out of memory)

During bulk loading of data, Dgraph can consume more memory than usual, due to high volume of writes. That's generally when you see the OOM crashes.

The recommended minimum RAM to run on desktops and laptops is 16GB. Dgraph can take up to 7-8 GB with the default setting `--lru_mb` set to 4096; so having the rest 8GB for desktop applications should keep your machine humming along.

On EC2/GCE instances, the recommended minimum is 8GB. It's recommended to set `--lru_mb` to one-third of RAM size.

You could also decrease memory usage of Dgraph by setting `--badger.vlog=disk`.

### Too many open files

If you see a log error messages saying `too many open files`, you should increase the per-process file descriptors limit.

During normal operations, Dgraph must be able to open many files. Your operating system may set by default a open file descriptor limit lower than what's needed for a database such as Dgraph.

On Linux and Mac, you can check the file descriptor limit with `ulimit -n -H` for the hard limit and `ulimit -n -S` for the soft limit. The soft limit should be set high enough for Dgraph to run properly. A soft limit of 65535 is a good lower bound for a production setup. You can adjust the limit as needed.

### Unauthorized IP address: X.X.X.X

To ensure security around admin operations, the default behaviour of admin operations has been changed.
 
Now by default, all the admin operations will be restricted from the alpha machine itself. Any admin operations outside the alpha environment will be rejected with unauthorized access error. 

You can use the `--whitelist` option to specify whitelisted IP addresses or ranges for hosts from which admin operations can be initiated. More about whitelisting IPs can be found [here]({{< relref "dgraph-administration.md" >}}).