#!/bin/bash
times="$1"

rm -rf outputs
mkdir outputs

# fix this for loop
for i in {0..2}
do
    bash test.sh > outputs/${i}.out
done
