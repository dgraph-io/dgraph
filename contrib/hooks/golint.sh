#!/bin/bash

if [ -z "$GOPATH" ]; then
	echo "ERROR: pre-commit hook for golint: \$GOPATH is empty."
	exit 1
fi

if [ -z "$(which golint)" ]; then
	echo "golint not found, please run: go get github.com/golang/lint/golint"
	exit 1
fi

# This script does not handle file names that contain spaces.
gofiles=$(git diff --cached --name-only --diff-filter=ACM | grep '\.go$' | grep -v '^vendor/')

errors=

# Run on one file at a time because a single invocation of golint
# with multiple files requires the files to all be in one package.
gofiles_with_warnings=()
echo -e "\033[32m Running golint on staged files. You can either acknowledge the warnings or step through them.\033[0m"
for gofile in $gofiles
do
	errcount=$(golint $gofile | wc -l)
	if [ "$errcount" -gt "0" ]; then
		errors=YES
		echo "$errcount suggestions for:"
		echo "golint $gofile"
		gofiles_with_warnings+=($gofile)
	fi
done

[ -z "$errors" ] && exit 0

# git doesn't give us access to user input, so let's steal it.
exec < /dev/tty
if [[ $? -eq 0 ]]; then
	# interactive shell. Prompt the user.
	echo
	echo "Lint suggestions were found. They're not enforced, but we're pausing"
	echo "to let you know before they get clobbered in the scrollback buffer."
	echo
	read -r -p 'Press enter to cancel, "s" to step through the warnings or type "ack" to continue: '
	if [ "$REPLY" = "ack" ]; then
		exit 0
	fi
	if [ "$REPLY" = "s" ]; then
		first_file="true"
		for gofile in "${gofiles_with_warnings[@]}"
		do
			echo
			if [ "$first_file" != "true" ]; then
				echo "Press enter to show the warnings for the next file."
				read
			fi
			golint $gofile
			first_file="false"
		done
	fi
else
	# non-interactive shell (e.g. called from Eclipse). Just display the errors.
	for gofile in "${gofiles_with_warnings[@]}"
	do
		golint $gofile
	done
fi
exit 1


