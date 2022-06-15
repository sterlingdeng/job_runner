# coding: utf-8
# -*- mode: ruby -*-
# vi: set ft=ruby :

vm_cpus = 1
vm_memory = 3492

Vagrant.configure(2) do |config|
  config.vm.box = "fedora/35-cloud-base"
  config.vm.provider "virtualbox" do |v, override|
    v.memory = vm_memory
    v.cpus = vm_cpus
    override.vm.synced_folder ".", "/home/vagrant", type: "virtualbox"
  end
  config.vm.provision "shell", inline: <<-SHELL
    set -eux -o pipefail
    dnf -y install gcc
    GO_VERSION="1.18.3"
    sudo sh -c "mkdir /lib_cgroup"
    sudo sh -c "mount -t cgroup2 none /lib_cgroup"

    curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" | tar Cxz /usr/local
    cat >> /etc/profile.d/sh.local <<EOF
PATH=/usr/local/go/bin:$PATH
EOF
    source /etc/profile.d/sh.local

   SHELL
end
