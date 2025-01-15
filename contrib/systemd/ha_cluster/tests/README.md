# Systemd Tests

These are tests to both demonstrate and test functionality of systemd units to manage dgraph.

## Requirements

- HashiCorp [Vagrant](https://www.vagrantup.com/) - automation to manage virtual machine systems and
  provision them.

## Instructions

### Create VM Guests and Provision

Either `cd centos8` or `cd ubuntu1804` and run:

```bash
vagrant up
```

#### Using Hyper/V Provider

On Windows 10 Pro with Hyper/V enabled, you can run this in PowerShell:

```powershell
$Env:VAGRANT_DEFAULT_PROVIDER = "hyperv"
vagrant up
```

#### Using libvirt Provider

If you running on Linux and would like to use KVM for a speedier Vagrant experience, you can install
the `vagrant-libvirt` plugin (see
[Installation](https://github.com/vagrant-libvirt/vagrant-libvirt#installation)) and run this:

```bash
export VAGRANT_DEFAULT_PROVIDER=libvirt
vagrant up
```

### Logging Into the System

You can log into the guest virtual machines with SSH.

```bash
vagrant ssh        # log into default `alpha-0`
vagrant status     # Get Status of running systems
vagrant ssh zero-1 # log into zero-1
```

### Get Health Check

You can check the health of a system with this pattern (using `awk` and `curl`):

```bash
# test a zero virtual guest
curl $(awk '/zero-0/{ print $1 }' hosts):6080/health
# test an alpha virtual guest
curl $(awk '/alpha-0/{ print $1 }' hosts):8080/health
```

### Get State of Cluster

You can check the state of the cluster with this pattern (using `awk` and `curl`):

```bash
# get state of cluster
curl $(awk '/zero-0/{ print $1 }' hosts):6080/state
```

### Get Logs

```bash
# get logs from zero0
vagrant ssh zero-0 --command "sudo journalctl -u dgraph-zero"
# get logs from alpha0
vagrant ssh alpha-0 --command "sudo journalctl -u dgraph-alpha"
```

### Cleanup and Destroy VMs

```bash
vagrant destroy --force
```

## About Automation

### Configuration

The configuration is a `hosts` file format, space-delimited. This defines both the hostnames and
virtual IP address used to create the virtual guests. Vagrant in combination with the underlying
virtual machine provider will create a virtual network accessible by the host.

```host
<inet_addr>   <hostname>
<inet_addr>   <hostname>   <default>
<inet_addr>   <hostname>
```

You can use `default` for one system to be designated as the default for `vagrant ssh`

#### Dgraph Version

By default, the latest Dgraph version will be used to for the version. If you want to use another
version, you can set the environment variable `DGRAPH_VERSION` for the desired version.

### Windows Environment

On Windows, for either Hyper/V or Virtualbox providers, for convenience you can specify username
`SMB_USER` and password `SMB_PASSWD` before running `vagrant up`, so that you won't get prompted 6
times for username and password.

> **NOTE**: Setting a password in an environment variable is not considered security best practices.

To use this in PowerShell, you can do this:

```powershell
$Env:SMB_USER = "<username>"   # example: $Env:USERNAME
$Env:SMB_PASSWD = "<password>"
# "hyperv" or "virtualbox"
$Env:VAGRANT_DEFAULT_PROVIDER = "<provider>"

vagrant up
```

## Environments Tested

- Guest OS
  - [Cent OS 8](https://app.vagrantup.com/generic/boxes/centos8) from
    [Roboxes](https://roboxes.org/)
  - [Ubuntu 18.04](https://app.vagrantup.com/generic/boxes/ubuntu1804) from
    [Roboxes](https://roboxes.org/)
- Providers
  - [libvirt](https://github.com/vagrant-libvirt/vagrant-libvirt) (KVM) on Ubuntu 19.10
  - [VirtualBox](https://www.vagrantup.com/docs/providers/virtualbox) on Win10 Home, Mac OS X 10.14
  - [Hyper/V](https://www.vagrantup.com/docs/providers/hyperv) on Win10 Pro

## Resources

- Vagrant
  - Util API: https://www.rubydoc.info/github/hashicorp/vagrant/Vagrant/Util/Platform
  - Multi-Machine: https://www.vagrantup.com/docs/multi-machine
  - Synced Folders: https://www.vagrantup.com/docs/synced-folders
    - lib-virt: https://github.com/vagrant-libvirt/vagrant-libvirt#synced-folders
  - Provisioning: https://www.vagrantup.com/docs/provisioning
- Dgraph
  - Documentation: https://dgraph.io/docs/
  - Community: https://discuss.dgraph.io/
