#!/bin/bash

# get the p directory
#wget https://storage.googleapis.com/dgraph-datasets/21million/p/p.tar.gz

#untar it
tar -xvf p.tar.gz 
# get the profiling and size
go test -run Test21MillionDataSet$ -v -manual=true

# compare our calculation with the profile
go test -run Test21MillionDataSetSize$ -v -manual=true

rm mem.out
rm size.data
rm -rf p 
rm  p.tar.gz
