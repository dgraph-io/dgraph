#!/bin/bash

# We run the TLS tests only when a commit is made on master/release
if [[ $TRAVIS_BRANCH =~ master|release\/ ]] && [ $TRAVIS_EVENT_TYPE = "push" ] ; then
	make test
fi
