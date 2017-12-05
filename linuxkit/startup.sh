#!/bin/bash
resize
mkdir /var/external
mount /dev/sda1 /var/external
chmod g+w /var/external
ln -s /var/external/containerd2 /var/lib
/var/external/mount-cgroups-rw.sh
echo "export CONTAINERD_NAMESPACE=examplectr" >/root/.bashrc
echo "alias ctr='ctr -a /run/containerd2/containerd.sock'" >>/root/.bashrc
echo "export PATH=$PATH:/var/external/usr/bin" >>/root/.bashrc
apk update && apk add tmux

