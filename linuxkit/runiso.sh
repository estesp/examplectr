#!/bin/sh

sudo linuxkit run qemu -uefi -iso -fw "/usr/share/ovmf/OVMF.fd" -disk lktest-efi-state/disk.img lktest-efi.iso
