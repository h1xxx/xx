.PHONY: xx tools

xx:
	CGO_ENABLED=0 go build -o xx/cntrun/cntrun xx/cntrun/cntrun.go
	cd xx/ && CGO_ENABLED=0 go build -o xx *.go

install: xx
	install -m 0750 -o root -g xx xx/xx /sbin/
	chown -R xx:xx /home/xx

tools:
	xx/xx build -f -s xx_tools_cross sys/busybox
	xx/xx build -f -s xx_tools_cross dev/musl
	xx/xx build -f -s xx_tools_cross sys/bash
	xx/xx build -f -s xx_tools_cross sys/file

bootstrap: clean_tmp bootstrap-cross bootstrap-cp bootstrap-final

bootstrap-cross:
	# cross-compiling compiler and libc from the host system and then
	# rebuilding them in environment isolated from the host system
	xx/xx build set/init_glibc.xx

bootstrap-cp: clean_tmp
	# creating environment only with packages build in isolation from the
	# host system; this step drops any leftover files from initial,
	# cross-compiled packages
	xx/xx build set/init_glibc_cp.xx
	mv /tmp/xx/init_glibc /tmp/xx/base
	touch /tmp/xx/base/bootstrap

bootstrap-final:
	# final build of all base packages
	xx/xx build set/base.xx
	bin/busybox sh -c 'chmod -fR +w /tmp/xx/ || :'
	rm -rf /tmp/xx/

bootstrap_musl: clean_tmp
	xx/xx b set/init_musl.xx
	mv /tmp/xx/init_musl/ /tmp/xx/musl_base
	rm -rf /tmp/xx/musl/{cross_tools,tools,usr}
	touch /tmp/xx/musl_base/base_ok
	xx/xx b set/musl_base.xx
	rm -rf /tmp/xx/

base:
	xx/xx build set/base.xx

all:
	xx/xx build set/dev.xx
	xx/xx build set/lxc.xx
	xx/xx build set/media/cd.xx
	xx/xx build set/media/gfx.xx
	xx/xx build set/media/sdl.xx
	xx/xx build set/media/snapcast.xx
	xx/xx build set/media/text.xx
	xx/xx build set/media/video.xx
	xx/xx build set/media/w3m.xx
	xx/xx build set/misc.xx
	xx/xx build set/net.xx
	xx/xx build set/qemu.xx
	xx/xx build set/sys.xx
	xx/xx build set/x11.xx

clean_tmp:
	bin/busybox sh -c 'chmod -fR +w /tmp/xx/ || :'
	rm -rf /tmp/xx/

clean_pkg:
	rm -fr prog/*/*/pkg/*/
	rm -fr prog/*/*/log/*/

	# bring back mime-types pkg
	git checkout prog/sys/mime-types/pkg/

clean_bootstrap:
	rm -fr prog/*/*/pkg/bootstrap_*cross-*/
	rm -fr prog/*/*/pkg/bootstrap-*/
	rm -fr prog/*/*/pkg/bootstrap_libstdcpp_2-*/

clean_src:
	rm -fr prog/*/*/src/*
