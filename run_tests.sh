#!/bin/bash
times="$1"

rm -rf outputs
mkdir outputs
for i in {0..$times}
do
    bash test.sh > outputs/$(i).out
done
