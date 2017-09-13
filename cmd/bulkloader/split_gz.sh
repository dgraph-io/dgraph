#!/bin/bash

set -e

if [[ $# != 2 ]]; then
		echo "Usage: $0 <input_dir> <output_dir>"
		exit 1
fi

inDir=$1
outDir=$2

rm -rf $outDir
mkdir $outDir
for inputFile in $inDir/*.rdf.gz; do
		echo Processing: $inputFile
		gunzip < $inputFile | split --lines=10000000 --filter='gzip > $FILE.gz &' - $outDir/
done
