# Create containers
#!/bin/sh
pwd
docker-compose --compatibility -f docker-compose.yml  up --force-recreate --build --remove-orphans --detach

sleep 300

echo "==== Showing docker logs"
docker logs s3-backup_zero1_1
docker logs s3-backup_alpha1_1
echo "====== Docker logs over"

#./healthCheck.sh s3-backup_zero1_1 120
#./healthCheck.sh s3-backup_zero2_1 120
#./healthCheck.sh s3-backup_alpha1_1 120
#./healthCheck.sh s3-backup_alpha2_1 120
#./healthCheck.sh s3-backup_alpha3_1 120
#./healthCheck.sh s3-backup_alpha4_1 120
#./healthCheck.sh s3-backup_alpha5_1 120
#./healthCheck.sh s3-backup_alpha6_1 120

docker ps -a
