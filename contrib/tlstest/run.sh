#!/bin/bash

# We run the TLS tests only when a commit is made on master/release
 if [ $TRAVIS_BRANCH =~ master|release\/ ] ; then
	make test
 else
 	echo "TLS tests skipped."
 fi
