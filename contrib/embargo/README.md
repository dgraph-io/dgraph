# embargo

## Install


### Installing Pyenv

On Ubuntu:

```bash
# install libraries and tools needed to build python
sudo apt-get install -y \
 libbz2-dev \
 liblzma-dev \
 llvm \
 make \
 python-openssl \
 tk-dev \
 wget \
 xz-utils

# install pyenv
PROJ=pyenv-installer
SCRIPT_URL=https://github.com/pyenv/$PROJ/raw/master/bin/$PROJ
curl -sL $SCRIPT_URL | bash

# configure current environment
export PATH="$HOME/.pyenv/bin:$PATH"
eval "$(pyenv init -)"
eval "$(pyenv virtualenv-init -)"

# configure shell environment
cat <<-'PYENV' > ~/.bashrc
export PATH="$HOME/.pyenv/bin:$PATH"
eval "$(pyenv init -)"
eval "$(pyenv virtualenv-init -)"
PYENV
```

### Installing Embargo

```bash
pyenv install 3.8.9
pyenv virtualenv 3.8.9 embargo-3.8.9
pyenv shell embargo-3.8.9
pip install embargo
```
