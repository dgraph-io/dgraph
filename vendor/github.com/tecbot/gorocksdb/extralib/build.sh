set -e

g++ -c -std=c++11 rocksdbextra.cc
ar q librocksdbextra.a rocksdbextra.o
rm -Rf rocksdbextra.o