#!/usr/bin/env bash
TEST_FAIL=0
# get the p directory
GCS_URL=https://storage.googleapis.com/dgraph-datasets/21million_test_p/p.tar.gz
wget --quiet $GCS_URL || { echo "ERROR: Download from '$GCS_URL' failed." >&2; exit 2; }

#untar it
[[ -f p.tar.gz ]] || { echo "ERROR: File 'p.tar.gz' does not exist. Exiting" >&2; exit 2; }
tar -xf p.tar.gz

# get the profiling and size
go test -run Test21MillionDataSet$ -v -manual=true
[[ $? -ne 0 ]] && TEST_FAIL=1

# compare our calculation with the profile
go test -run Test21MillionDataSetSize$ -v -manual=true
[[ $? -ne 0 ]] && TEST_FAIL=1

# cleanup (idempotent)
rm -f mem.out
rm -f size.data
rm -rf p
rm -f p.tar.gz

# report to calling script that test passed or failed
if [[ $TEST_FAIL -ne 0 ]]; then
  exit 1
else
  exit 0
fi
