#!/bin/bash

set -e

# You will need the flatbuffer compiler (flatc) https://google.github.io/flatbuffers/flatbuffers_guide_building.html
# TODO: If flatc is not present, this should download the release, compile it and install it.

flatc --go ./schema.fbs

# Move files to the correct directory.
mv fb/* ./
rmdir fb
