#!/usr/bin/env sh
#
# This script is meant to be used to update the version of rocksdb in this repo.
#
# To use it, update the version number in the URL to the tar.gz file below, then
# run it, starting from a clean repo. If you're lucky, everything will just
# work, but you may get an error about a patch not applying cleanly. If that's
# the case, try to fix up the patch file so that it will apply cleanly, then
# reset the rest of the repository and try again.
#
# Once things have run cleanly, you should have a fairly large diff of the
# actual rocksdb code, potentially along with some added or removed symlink
# files in the base of the repository.
#
# Once you've successfully run this script, you should also manually verify that
# the recommended build flags for darwin and linux mostly match up with what's
# specified in the cgo_flags.go file in the base of the repo.
# To do so go through the below process on both a mac machine and a linux
# machine (or a linux docker container):
#   cd internal
#   ./build_tools/build_detect_platform tmp.txt
#   # Manually update cgo_flags.go with PLATFORM_{CC,CXX}FLAGS from tmp.txt.
#   #
#   # Note that musl doesn't support some features of glibc, so we omit:
#   # -DROCKSDB_PTHREAD_ADAPTIVE_MUTEX -DROCKSDB_BACKTRACE
#   # on Linux.
#   #
#   # Note that Linux 2.6.32 (RHEL 6) doesn't support FALLOC_FL_PUNCH_HOLE so we omit:
#   # -DROCKSDB_FALLOCATE_PRESENT
#   # on Linux.
#
# Ask @tamird if you run into issues along the way.

set -eu

rm -rf internal/*
find . -type l -not -path './.git/*' -exec rm {} \;
curl -sL https://github.com/facebook/rocksdb/archive/v5.1.4.tar.gz | tar zxf - -C internal --strip-components=1
# TODO(tamird): remove when
# https://github.com/facebook/rocksdb/pull/1910 is merged and released.
patch -d internal -p1 < gettimeofday.patch
# TODO(tamird): remove when
# https://github.com/facebook/rocksdb/pull/1931 is merged and released.
patch -d internal -p1 < abort.patch
# Downcase some windows-only includes for compatibility with mingw64.
grep -lR '^#include <.*[A-Z].*>' internal | while IFS= read -r source_file; do
  awk '/^#include <.*[A-Z].*>/ { print tolower($0); next; } { print; }' "$source_file" > tmp
  mv tmp "$source_file"
done
# Avoid MSVC-only extensions for compatibility with mingw64.
grep -lRF 'i64;' internal | xargs sed -i~ 's!i64;!LL;!g'

make -C internal util/build_version.cc

# symlink so cgo compiles them
for source_file in $(make sources | grep -vE '^internal/(port/win|utilities/redis)/|_posix.cc$'); do
  ln -sf "$source_file" "$(echo "$source_file" | tr / _)"
done

git clean -dXf
