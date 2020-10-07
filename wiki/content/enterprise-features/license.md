+++
date = "2017-03-20T22:25:17+11:00"
title = "License"
weight = 4
[menu.main]
    parent = "enterprise-features"
+++

Dgraph enterprise features are proprietary licensed under the [Dgraph Community
License][dcl]. All Dgraph releases contain proprietary code for enterprise features.
Enabling these features requires an enterprise contract from
[contact@dgraph.io](mailto:contact@dgraph.io) or the [discuss
forum](https://discuss.dgraph.io).

**Dgraph enterprise features are enabled by default for 30 days in a new cluster**.
After the trial period of thirty (30) days, the cluster must obtain a license from Dgraph to
continue using the enterprise features released in the proprietary code.

{{% notice "note" %}}
At the conclusion of your 30-day trial period if a license has not been applied to the cluster,
access to the enterprise features will be suspended. The cluster will continue to operate without
enterprise features.
{{% /notice %}}

When you have an enterprise license key, the license can be applied to the cluster by including it
as the body of a POST request and calling `/enterpriseLicense` HTTP endpoint on any Zero server.

```sh
curl -X POST localhost:6080/enterpriseLicense --upload-file ./licensekey.txt
```

It can also be applied by passing the path to the enterprise license file (using the flag
`--enterprise_license`) to the `dgraph zero` command used to start the server. The second option is
useful when the process needs to be automated.

```sh
dgraph zero --enterprise_license ./licensekey.txt
```

[dcl]: https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt