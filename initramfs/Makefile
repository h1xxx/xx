.PHONY: initramfs

initramfs:
	rm -rf build
	mkdir -p build/tmp
	mkdir -p build/ramfs_root/{bin,etc,mnt/boot,mnt/root,root,share}
	mkdir -p build/ramfs_root/{dev,proc,run,sys,tmp}

	# install pkgs to a temporary dir
	../xx/xx install -r /home/xx/initramfs/build/tmp initramfs.xx

	# create a final initramfs system from tmp dir
	cd build/tmp && cp bin/busybox \
		bin/dmesg \
		bin/findmnt \
		bin/lsblk \
		bin/lsirq \
		bin/lsmem \
		bin/scp \
		bin/ssh \
		bin/ssh-keygen \
		sbin/blkid \
		sbin/badblocks \
		sbin/blockdev \
		sbin/cfdisk \
		sbin/cryptsetup \
		sbin/ctrlaltdel \
		sbin/dhcpcd \
		sbin/fdisk \
		sbin/findfs \
		sbin/fsck \
		sbin/fsck.ext4 \
		sbin/fstrim \
		sbin/sfdisk \
		sbin/sshd \
		sbin/switch_root \
		sbin/uuidd \
		../../build/ramfs_root/bin
	cp -a build/tmp/etc build/ramfs_root/

	cd build/ramfs_root/bin && ./busybox --install .
	ln -sf busybox build/ramfs_root/bin/sh

	cp -a files/init build/ramfs_root/
	cp -a files/ash.rc build/ramfs_root/root/
	cp -a files/unifont-apl8x16.psf.gz build/ramfs_root/share/

	ssh-keygen -N '' -t ed25519 \
		-f build/ramfs_root/etc/ssh/ssh_host_ed25519_key || :

	# creating cpio file
	cd build/ramfs_root && find * -print0 | \
		cpio --owner=root:root --null --create --format=newc | \
		/home/xx/bin/zstd -19 -T4 --long=27 > ../initramfs.cpio.zst

