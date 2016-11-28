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
#   # Manually compare the PLATFORM_CCFLAGS and PLATFORM_CXXFLAGS to what's in
#   # cgo_flags.go
#
# Ask @tamird if you run into issues along the way.
#
# After committing locally you should run the command below to ensure your repo
# is in a clean state and then build/test cockroachdb with the new version:
#   git clean -dxf

set -eu

rm -rf internal/*
find . -type l -not -path './.git/*' | xargs rm
curl -sL https://github.com/facebook/rocksdb/archive/v4.11.2.tar.gz | tar zxf - -C internal --strip-components=1
make -C internal util/build_version.cc
patch -p1 < gitignore.patch
patch -p1 < testharness.patch
# Downcase some windows-only includes for compatibility with mingw64.
grep -lRF '<Windows.h>' internal | xargs sed -i~ 's!<Windows.h>!<windows.h>!g'
grep -lRF '<Rpc.h>' internal | xargs sed -i~ 's!<Rpc.h>!<rpc.h>!g'
# Avoid MSVC-only extensions for compatibility with mingw64.
grep -lRF 'i64;' internal | xargs sed -i~ 's!i64;!LL;!g'

# symlink so cgo compiles them
for source_file in $(make sources | grep -vE '(/redis/|(_(cmd|tool)|(env|port)_[a-z]+).cc$)'); do
  ln -sf $source_file $(echo $source_file | sed s,/,_,g)
done
