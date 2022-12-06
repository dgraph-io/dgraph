# Create containers
#!/bin/sh
pwd
docker-compose --compatibility -f docker-compose.yml  up --force-recreate --build --remove-orphans --detach
./healthCheck.sh zero1_1 120
./healthCheck.sh zero2_1 120
./healthCheck.sh alpha1_1 120
./healthCheck.sh alpha2_1 120
.healthCheck.sh alpha3_1 120
./healthCheck.sh alpha4_1 120
./healthCheck.sh alpha5_1 120
./healthCheck.sh alpha6_1 120
