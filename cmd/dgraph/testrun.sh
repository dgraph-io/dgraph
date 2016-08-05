#!/bin/bash

set -e

dir="$HOME/dgraph"
i=0;

if ! hash dgraph 2>/dev/null; then
	echo "Please install Dgraph and try again."
  exit 1
fi

# Double quotes are used to store the command in a variable which can be used later. `${i}` is how you access value of a variable in a double quoted string. Also other double quotes have to be escaped like for workers with a double quoted string. Also tee is used in append mode to redirect out to log file apart from displaying it on stdout.
server1="dgraph --instanceIdx '${i}' --mutations $dir/m'${i}' --port 8080 --postings $dir/p'${i}' --workers \":12345,:12346\" --uids $dir/u --workerport \":12345\" --stw_ram_mb=6000 2>&1 | tee -a dgraph.log &"
i=1;
server2="dgraph --instanceIdx '${i}' --mutations $dir/m'${i}' --port 8082 --postings $dir/p'${i}' --workers \":12345,:12346\" --uids $dir/u --workerport \":12346\" 2>&1 | tee -a dgraph.log &"

function checkServer {
	port=$1

	# Status evaluates if there is a process running on $port.
	status=$(nc -z 127.0.0.1 $port; echo $?)

	# If status is 1, we restart the relevant server
	if [ $status -ne 0 ]; then
	 if [ $port -eq "8080" ]; then
		 echo "Restarting server 1"
		 eval $server1
	 fi
	 if [ $port -eq "8082" ]; then
		 echo "Restarting server 2"
		 eval $server2
	 fi
	fi
}

# Kill already running Dgraph processes.
if pgrep "dgraph" > /dev/null; then
	killall dgraph
fi

# Start the servers.
echo "Starting server 1"
eval $server1
echo "Starting server 2"
eval $server2

# Check that the servers should be running every 30 seconds.
while true; do
	sleep 30
	checkServer 8080
	checkServer 8082
done
