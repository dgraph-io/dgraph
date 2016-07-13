#!/usr/bin/env sh

set -eu

rm -rf internal/*
find . -type l -not -path './.git/*' | xargs rm
curl -sL https://github.com/facebook/rocksdb/archive/v4.3.1.tar.gz | tar zxf - -C internal --strip-components=1
make -C internal util/build_version.cc
patch -p1 < gitignore.patch
patch -p1 < internal.patch
patch -p1 < iterate-upper-bound.patch
patch -p1 < prefix-same-as-start.patch

# symlink so cgo compiles them
for source_file in $(make sources | grep -vE '(/redis/|(_(cmd|tool)|(env|port)_[a-z]+).cc$)'); do
  ln -sf $source_file $(echo $source_file | sed s,/,_,g)
done

# manually use internal/build_tools/build_detect_platform to update the per-
# platform flags in cgo_flags.

# restore the repo to what it would look like when first cloned.
# comment this line out while updating upstream.
git clean -dxf
