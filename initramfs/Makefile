.PHONY: initramfs

initramfs:
	mkdir -p build/tmp
	rm -rf build/tmp/* build/initramfs.cpio.zst

	mkdir -p build/tmp/root

	../xx/xx install -r /home/xx/initramfs/build/tmp initramfs.xx
	
	# remove unnecessary files
	rm -rf build/tmp/etc/ssl
	rm -rf build/tmp/usr/share
	rm -rf build/tmp/usr/include
	rm -rf build/tmp/usr/lib/gconv
	rm -rf build/tmp/usr/lib/locale
	rm -rf build/tmp/usr/lib/*.a

	mkdir -p build/tmp/usr/share/consolefonts
	cp -a files/init build/tmp/
	cp -a files/ash.rc build/tmp/root/
	cp -a files/unifont-apl8x16.psf.gz build/tmp/usr/share/consolefonts/

	ssh-keygen -N '' -t ed25519 -f build/tmp/etc/ssh/ssh_host_ed25519_key ||:

	cd build/tmp && find * -print0 | \
		cpio --owner=root:root --null --create --format=newc | \
		zstd -19 -T8 > ../initramfs.cpio.zst

