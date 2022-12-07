#!/bin/bash
container_name="$1"
# Timeout in seconds. Default: 60
timeout=$((${2:-60}));

if [ -z $container_name ]; then
  echo "No container name specified";
  exit 1;
fi

echo "Container: $container_name";
echo "Timeout: $timeout sec";

try=0;
is_healthy="false";
while [ $is_healthy != "running" ];
do
  try=$(($try + 1));
  printf "■";
  is_healthy=$(docker inspect --format='{{json .State.Status}}' $container_name);
  echo " Container status "
  echo "Container $container_name: $is_healthy"
  sleep 1;
  if [[ $try -eq $timeout ]]; then
    echo " Container $container_name did not boot within timeout";
    exit 1;
  fi
done

