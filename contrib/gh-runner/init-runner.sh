#!/bin/bash
# Get GH Actions Runner registration token
export AWS_REGION=us-east-1
AWS_INSTANCE_ID=$(curl -s http://169.254.169.254/latest/meta-data/instance-id)
EC2_INSTANCE_NAME=$(aws ec2 describe-tags --filters "Name=resource-id,Values=$AWS_INSTANCE_ID" "Name=key,Values=Name" --output text | cut -f5)
TOKEN=$(aws ssm get-parameter --name $EC2_INSTANCE_NAME --with-decryption | jq -r '.Parameter.Value')
sudo -i -u ubuntu bash << EOF
# Install & Setup GH Actions Runner
cd /home/ubuntu
mkdir actions-runner && cd actions-runner
curl -o actions-runner-linux-x64-2.296.2.tar.gz -L https://github.com/actions/runner/releases/download/v2.296.2/actions-runner-linux-x64-2.296.2.tar.gz
echo "34a8f34956cdacd2156d4c658cce8dd54c5aef316a16bbbc95eb3ca4fd76429a  actions-runner-linux-x64-2.296.2.tar.gz" | shasum -a 256 -c
tar xzf ./actions-runner-linux-x64-2.296.2.tar.gz
./config.sh --unattended --url https://github.com/dgraph/dgraph --token $TOKEN --name $EC2_INSTANCE_NAME
# Delete token
aws ssm delete-parameter --name $EC2_INSTANCE_NAME
# Start GH Actions
sudo ./svc.sh install
sudo ./svc.sh start
# Reboot Machine
sudo reboot
EOF
