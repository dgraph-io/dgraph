# GH Actions Runner

### Automatic Setup
We use [Packer](https://www.packer.io) to create an AWS automated machine image (AMI) for our EC2 self-hosted runners. 
This machine image contains all the dependencies (based on `installl-deps.sh`) that we need to run a Github actions job. It will also
register itself as a runner, which is done by `init-runner.sh` which we put in `/var/lib/cloud/scripts/per-instance`
so that it runs when an instance is first booted. 

In summary, every instance that is created from this AMI is ready to run a Github actions job without any further configurations.

How to build the AMI?

1. [Install](https://developer.hashicorp.com/packer/tutorials/aws-get-started/get-started-install-cli) Packer on your machine
2. [Authenticate](https://developer.hashicorp.com/packer/plugins/builders/amazon#authentication) to AWS
3. Build the AMI
```shell
packer build . -var "<VARIABLE_NAME>=<VARIABLE_VALUE>"
```
4. Visit the [AWS AMI](https://us-east-1.console.aws.amazon.com/ec2/home?region=us-east-1#Images:visibility=owned-by-me;sort=imageName) page to verify that Packer successfully built your AMI.


### Manual Setup
How to bake an AWS Virtual Machine for GH Actions?

1. Get a Github Actions Runner Token from the UI
```
export TOKEN=<GITHUB ACTIONS RUNNER TOKEN>
```
2. Download the setup script onto the machine
```
wget https://raw.githubusercontent.com/dgraph-io/dgraph/main/contrib/gh-runner/gh-runner.sh
```
3. Run the script to attach this machine to Github Actions Runner Pool
```
sh gh-runner.sh
```
NOTE: this will reboot the machine, once the machine is back up it will connect to Github
