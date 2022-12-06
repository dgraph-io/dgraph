# Create containers
#!/bin/sh
pwd
docker-compose --compatibility -f docker-compose.yml  up --force-recreate --build --remove-orphans --detach
healthcheck.sh zero1_1 120
healthcheck.sh zero2_1 120
healthcheck.sh alpha1_1 120
healthcheck.sh alpha2_1 120
healthcheck.sh alpha3_1 120
healthcheck.sh alpha4_1 120
healthcheck.sh alpha5_1 120
healthcheck.sh alpha6_1 120
