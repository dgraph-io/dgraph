# GH Actions Runner

How to bake an AWS Virtual Machine for GH Actions?

1. Get a Github Actions Runner Token from the UI
```
export TOKEN=<GITHUB ACTIONS RUNNER TOKEN>
```
2. Download the setup script onto the machine
```
wget https://raw.githubusercontent.com/adityasadalage/dgraph/main/contrib/gh-runner/gh-runner.sh
```
3. Run the script to attach this machine to Github Actions Runner Pool
```
sh gh-runner.sh
```
NOTE: this will reboot the machine, once the machine is back up it will connect to Github
