+++
date = "2017-03-20T22:25:17+11:00"
title = "Log Format"
weight = 9
[menu.main]
    parent = "deploy"
+++

Dgraph's log format comes from the glog library and is [formatted](https://github.com/golang/glog/blob/23def4e6c14b4da8ac2ed8007337bc5eb5007998/glog.go#L523-L533) as follows:

	`Lmmdd hh:mm:ss.uuuuuu threadid file:line] msg...`

Where the fields are defined as follows:

```
	L                A single character, representing the log level (eg 'I' for INFO)
	mm               The month (zero padded; ie May is '05')
	dd               The day (zero padded)
	hh:mm:ss.uuuuuu  Time in hours, minutes and fractional seconds
	threadid         The space-padded thread ID as returned by GetTID()
	file             The file name
	line             The line number
	msg              The user-supplied message
```

## Query Logging

To enable query logging, you must set `-v=3` which will enable verbose logging for everything. Alternatively, you can set `--vmodule=server=3` for only the dgraph/server.go file which would only enable query/mutation logging.
