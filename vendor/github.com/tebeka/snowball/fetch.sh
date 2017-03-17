#!/bin/bash
# Fetch snowball sources

curl -LO http://snowball.tartarus.org/dist/libstemmer_c.tgz
tar -xzf libstemmer_c.tgz
find libstemmer_c -name '*.[ch]' -exec cp {} . \;
rm stemwords.c libstemmer_utf8.c # example and duplicate
rm -rf libstemmer_c libstemmer_c.tgz
sed -i 's|include "../[a-z_]\+/|include "|' *.{c,h}
