#!/bin/sh

# generate the tables we need
go run gen.go -table=ssemasks |gofmt >ssemasks.go
go run gen.go -table=bytesused |sed 's/ $//' >bytesused.go
