#!/bin/bash

set -e

dir="$HOME/dgraph"

# We display error to the user if Dgraph isn't installed.
if ! hash dgraph 2>/dev/null; then
	echo "Please install Dgraph and try again."
  exit 1
fi

# Double quotes are used to store the command in a variable which can be used later. `${i}` is how you access value of a variable in a double quoted string. Also other double quotes have to be escaped like for workers with a double quoted string. Also tee is used in append mode to redirect out to log file apart from displaying it on stdout.
i=1;
server1="dgraph --config testrun/conf1.yaml 2>&1 | tee -a dgraph1.log &"
i=2;
server2="dgraph --config testrun/conf2.yaml 2>&1 | tee -a dgraph2.log &"
i=3;
server3="dgraph --config testrun/conf3.yaml 2>&1 | tee -a dgraph3.log &"

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
	 if [ $port -eq "8084" ]; then
		 echo "Restarting server 3"
		 eval $server3
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
# Lets wait for the first server to bootup because it will form the cluster.
sleep 5
echo "Starting server 2"
eval $server2
sleep 5
echo "Starting server 3"
eval $server3

# Check that the servers should be running every 30 seconds.
while true; do
	sleep 10
	checkServer 8080
	checkServer 8082
	checkServer 8084
done
