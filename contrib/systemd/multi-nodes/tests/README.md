# SystemD Tests

These are tests to both demonstrate and test functionality of systemd units to managed dgraph.

## Requirements

* HashiCorp [Vagrant](https://www.vagrantup.com/) - automation to manage virtual machine systems and provision them.

## Instructions

### Creates VM Guests and Provision

Either `cd centos8` or `cd ubuntu1804` and run:

```bash
vagrant up
# test a zero virtual guest
curl $(awk '/zero0/{ print $1 }' hosts):6080/health
# test an alpna virtual guest
curl $(awk '/alpha0/{ print $1 }' hosts):8080/health

# get state of cluster
curl $(awk '/zero0/{ print $1 }' hosts):6080/state
```

### Cleanup and Destroy VMs

```bash
vagrant destroy --force
```


## Tested Environments

* Guest OS
  * Cent OS 8
  * Ubuntu 18.04
* Providers
  * libvrt (KVM)
  * VirtualBox on Win10 Home
