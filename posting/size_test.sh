#!/bin/bash

# get the profiling and size
go test -run Test21MillionDataSet -v -manual=true

# compare oru calculation with the profile
go test -run Test21MillionDataSetSize -v -manual=true

rm mem.out
rm size.data