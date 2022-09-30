### This script is for installing Go and all dependencies necessary
### to build Dgraph.  This may be useful for testing when starting a fresh
### EC2 instance, e.g.
### N.B. This script is currently not idempotent.

# Go setup
# If go is not installed, install it
if [ ! -d /usr/local/go ] ; then \
sudo curl -OL https://go.dev/dl/go1.19.1.linux-amd64.tar.gz ; \
sudo tar -C /usr/local/ -xzf go1.19.1.linux-amd64.tar.gz ; \
rm -f go1.19.1.linux-amd64.tar.gz ; \
fi

# Configure .bashrc
# NB this will be sourced only after the script returns
cat <<-'EOF' >> ~/.bashrc

export PATH=$PATH:/usr/local/go/bin
export PATH=$PATH:$(go env GOPATH)/bin
export GOPATH=$(go env GOPATH)
EOF

# Install dependencies
sudo apt-get update
sudo apt-get -y upgrade
sudo apt-get -y install \
    ca-certificates \
    curl \
    gnupg \
    lsb-release \
    build-essential \
    protobuf-compiler \

# Docker Setup
sudo mkdir -p /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
 echo \
   "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
   $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
sudo apt-get -y update
sudo apt-get -y install docker-ce docker-ce-cli containerd.io docker-compose-plugin
sudo apt-get -y install docker-compose

# Add Docker to sudo users group
sudo usermod -aG docker ${USER}
newgrp docker

# Now you are ready to test Dgraph
# mkdir -p $GOPATH/bin
# git clone https://github.com/dgraph-io/dgraph.git
# cd ./dgraph
# make test
