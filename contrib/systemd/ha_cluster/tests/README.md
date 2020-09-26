# Systemd Tests

These are tests to both demonstrate and test functionality of systemd units to manage dgraph.

## Requirements

* HashiCorp [Vagrant](https://www.vagrantup.com/) - automation to manage virtual machine systems and provision them.

## Instructions

### Create VM Guests and Provision

Either `cd centos8` or `cd ubuntu1804` and run:

```bash
vagrant up
```

#### Hyper/V Provider

On Windows 10 Pro with Hyper/V enabled, you can run this:

```powershell
$Env:VAGRANT_DEFAULT_PROVIDER = "hyperv"
vagrant up
```

### Get Health Check

```bash
# test a zero virtual guest
curl $(awk '/zero-0/{ print $1 }' hosts):6080/health
# test an alpha virtual guest
curl $(awk '/alpha-0/{ print $1 }' hosts):8080/health
```

### Get State of Cluster

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

## Windows Users

On Windows, for either Hyper/V or Virtualbox providers, for convenience you can specify username `SMB_USER` and password `SMB_PASSWD` before running `vagrant up`, so that you won't get prompted 6 times for username and password.  **NOTE**: That setting password as environment variable is not considered secure.

```powershell
$Env:SMB_USER = "<username>"   # e.g. $Env:USERNAME
$Env:SMB_PASSWD = "<password>"
$Env:VAGRANT_DEFAULT_PROVIDER = "<provider>" # e.g. "hyperv", "virtualbox"
vagrant up
```

## Environments Tested

* Guest OS
  * [Cent OS 8](https://app.vagrantup.com/generic/boxes/centos8) from [Roboxes](https://roboxes.org/)
  * [Ubuntu 18.04](https://app.vagrantup.com/generic/boxes/ubuntu1804) from [Roboxes](https://roboxes.org/)
* Providers
  * [libvirt](https://github.com/vagrant-libvirt/vagrant-libvirt) (KVM) on Ubuntu 19.10
  * [VirtualBox](https://www.vagrantup.com/docs/providers/virtualbox) on Win10 Home
  * [Hyper/V](https://www.vagrantup.com/docs/providers/hyperv) on Win10 Pro

## Resources

* Vagrant API: https://www.rubydoc.info/github/hashicorp/vagrant/Vagrant/Util/Platform
