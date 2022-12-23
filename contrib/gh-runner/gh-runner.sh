#!/bin/bash
# Machine Setup
sudo apt-get update
sudo apt-get -y upgrade
sudo apt-get -y install \
    ca-certificates \
    curl \
    gnupg \
    lsb-release \
    nfs-kernel-server\
    build-essential #Libc Packages
# Docker Setup
sudo mkdir -p /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
 echo \
   "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
   $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
sudo apt-get -y update
sudo apt-get -y install docker-ce docker-ce-cli containerd.io docker-compose-plugin
sudo apt-get -y install docker-compose
#Add Docker to sudoers group
sudo usermod -aG docker ${USER}
newgrp docker &
# Install & Setup GH Actions Runner
mkdir actions-runner && cd actions-runner
if [ "$(uname -m)" = "aarch64" ]; then
    echo "Detected arm64 architecture"
    # Download the latest runner package
    curl -o actions-runner-linux-arm64-2.298.2.tar.gz -L https://github.com/actions/runner/releases/download/v2.298.2/actions-runner-linux-arm64-2.298.2.tar.gz
    # Optional: Validate the hash
    echo "803e4aba36484ef4f126df184220946bc151ae1bbaf2b606b6e2f2280f5042e8  actions-runner-linux-arm64-2.298.2.tar.gz" | shasum -a 256 -c
    # Extract the installer
    tar xzf ./actions-runner-linux-arm64-2.298.2.tar.gz
elif [ "$(uname -m)" = "x86_64" ]; then
    echo "Detected amd64 architecture"
    # Download the latest runner package
    curl -o actions-runner-linux-x64-2.298.2.tar.gz -L https://github.com/actions/runner/releases/download/v2.298.2/actions-runner-linux-x64-2.298.2.tar.gz
    # Optional: Validate the hash
    echo "0bfd792196ce0ec6f1c65d2a9ad00215b2926ef2c416b8d97615265194477117  actions-runner-linux-x64-2.298.2.tar.gz" | shasum -a 256 -c 
    # Extract the installer
    tar xzf ./actions-runner-linux-x64-2.298.2.tar.gz
else
    echo "Unrecognized architecture"
    exit 1
fi
# Create the runner and start the configuration experience
./config.sh --url https://github.com/dgraph-io/dgraph --token $TOKEN
# CI Permission Issue
sudo touch /etc/cron.d/ci_permissions_resetter
sudo chown $USER:$USER /etc/cron.d/ci_permissions_resetter
sudo echo "* * * * * root for i in {0..60}; do chown -R $USER:$USER /home/ubuntu/actions-runner/_work & sleep 1; done" >  /etc/cron.d/ci_permissions_resetter 
sudo chown root:root /etc/cron.d/ci_permissions_resetter 
# Increase file descriptor limit
sudo sh -c 'echo "*         hard    nofile      2000000" >> /etc/security/limits.conf'
sudo sh -c 'echo "*         soft    nofile      2000000" >> /etc/security/limits.conf'
sudo sh -c 'echo "root      hard    nofile      2000000" >> /etc/security/limits.conf'
sudo sh -c 'echo "root      soft    nofile      2000000" >> /etc/security/limits.conf'
sudo sh -c 'echo "fs.nr_open = 2000000" >> /etc/sysctl.conf'
# Start GH Actions
sudo ./svc.sh install
sudo ./svc.sh start
# Reboot Machine
sudo reboot
