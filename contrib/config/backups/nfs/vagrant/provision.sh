#!/usr/bin/env bash

######
## main
#################################
main() {
  export DEV_USER=${1:-'vagrant'}
  export PYTHON_VERSION=${PYTHON_VERSION:-'3.8.2'}
  INSTALL_DOCKER=${INSTALL_DOCKER:-'true'}
  INSTALL_COMPOSE=${INSTALL_COMPOSE:-'true'}

  setup_hosts

  case $(hostname) in
    *nfs-server*)
      install_nfs_server
      ;;
    *nfs-client*)
      install_nfs_client
      [[ $INSTALL_DOCKER =~ "true" ]] && install_docker
      [[ $INSTALL_COMPOSE =~ "true" ]] && \
        export -f install_compose && \
        install_common && \
        su $DEV_USER -c "install_compose"
      ;;
  esac

}

######
## setup_hosts - configure /etc/hosts in absence of DNS
#################################
setup_hosts() {
  CONFIG_FILE=/vagrant/hosts
  if [[ ! -f /vagrant/hosts ]]; then
    echo "INFO: '$CONFIG_FILE' does not exist. Skipping configuring /etc/hosts"
    return 1
  fi

  while read -a LINE; do
    ## append to hosts entry if it doesn't exist
    if ! grep -q "${LINE[1]}" /etc/hosts; then
      printf "%s %s \n" ${LINE[*]} >> /etc/hosts
    fi
  done < $CONFIG_FILE
}

######
## install_nfs_server
#################################
install_nfs_server() {
  SHAREPATH=${1:-"/srv/share"}
  ACCESSLIST=${2:-'*'}
  apt-get -qq update && apt-get install -y nfs-kernel-server
  mkdir -p $SHAREPATH
  chown -R nobody:nogroup $SHAREPATH
  chmod -R 777 $SHAREPATH
  sed -i "\:$SHAREPATH:d" /etc/exports
  echo "$SHAREPATH    $ACCESSLIST(rw,sync,no_root_squash,no_subtree_check)" >> /etc/exports
  exportfs -rav
}

######
## install_nfs_client
#################################
install_nfs_client() {
  MOUNTPATH=${1:-"/mnt/share"}
  NFS_PATH=${2:-"/srv/share"}
  NFS_SERVER=$(grep nfs-server /vagrant/vagrant/hosts | cut -d' ' -f1)
  apt-get -qq update && apt-get install -y nfs-common

  mkdir -p $MOUNTPATH
  mount -t nfs $NFS_SERVER:$NFS_PATH $MOUNTPATH
}

######
## install_common
#################################
install_common() {
  apt-get update -qq -y

  ## tools and libs needed by pyenv
  ## ref. https://github.com/pyenv/pyenv/wiki/Common-build-problems
  apt-get install -y \
  build-essential \
  curl \
  git \
  libbz2-dev \
  libffi-dev \
  liblzma-dev \
  libncurses5-dev \
  libncursesw5-dev \
  libreadline-dev \
  libsqlite3-dev \
  libssl-dev \
  llvm \
  make \
  python-openssl \
  software-properties-common \
  sqlite \
  tk-dev \
  wget \
  xz-utils \
  zlib1g-dev
}

######
## install_docker
#################################
install_docker() {
  [[ -z "$DEV_USER" ]] && { echo '$DEV_USER not specified. Aborting' 2>&1 ; return 1; }

  apt update -qq -y && apt-get install -y \
   apt-transport-https \
   ca-certificates \
   gnupg-agent

  curl -fsSL https://download.docker.com/linux/ubuntu/gpg | apt-key add -
  add-apt-repository \
   "deb [arch=amd64] https://download.docker.com/linux/ubuntu \
   $(lsb_release -cs) \
   stable"
  apt update -qq -y
  apt-get -y install docker-ce docker-ce-cli containerd.io

  usermod -aG docker $DEV_USER
}

######
## install_compose - installs pyenv, python, docker-compose
#################################
install_compose() {
  PROJ=pyenv-installer
  SCRIPT_URL=https://github.com/pyenv/$PROJ/raw/master/bin/$PROJ
  curl -sL $SCRIPT_URL | bash

  ## setup current environment
  export PATH="$HOME/.pyenv/bin:$PATH"
  eval "$(pyenv init -)"
  eval "$(pyenv virtualenv-)"

  ## append to shell environment
  cat <<-'BASHRC' >> ~/.bashrc

export PATH="$HOME/.pyenv/bin:$PATH"
eval "$(pyenv init -)"
eval "$(pyenv virtualenv-init -)"
BASHRC

  ## install recent version of python 3
  pyenv install $PYTHON_VERSION
  pyenv global $PYTHON_VERSION
  pip install --upgrade pip
  pip install docker-compose
  pyenv rehash
}

main $@
