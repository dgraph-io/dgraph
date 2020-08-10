# SystemD Tests

These are tests to both demonstrate and test functionality of systemd units to manage dgraph.

## Requirements

* HashiCorp [Vagrant](https://www.vagrantup.com/) - automation to manage virtual machine systems and provision them.

## Instructions

### Create VM Guests and Provision

Either `cd centos8` or `cd ubuntu1804` and run:

```bash
vagrant up
```

### Get Health Check

```bash
# test a zero virtual guest
curl $(awk '/zero0/{ print $1 }' hosts):6080/health
# test an alpna virtual guest
curl $(awk '/alpha0/{ print $1 }' hosts):8080/health
```

### Get State of Cluster

```bash
# get state of cluster
curl $(awk '/zero0/{ print $1 }' hosts):6080/state
```

### Get Logs

```bash
# get logs from zero0
vagrant ssh zero0 --command "sudo journalctl -u dgraph-zero"
# get logs from alpha0
vagrant ssh alpha0 --command "sudo journalctl -u dgraph-alpha"
```
### Cleanup and Destroy VMs

```bash
vagrant destroy --force
```

## Environments Tested

* Guest OS
  * Cent OS 8
  * Ubuntu 18.04
* Providers
  * libvrt (KVM)
  * VirtualBox on Win10 Home
