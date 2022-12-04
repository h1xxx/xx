.PHONY: xx tools bootstrap base clean-tmp

xx:
	CGO_ENABLED=0 go build -o xx/cntrun/cntrun xx/cntrun/cntrun.go
	cd xx/ && CGO_ENABLED=0 go build -o xx *.go

install: xx
	install -m 0750 -o root -g xx xx/xx /sbin/
	chown -R xx:xx /home/xx

tools:
	xx/xx build -f -s xx_tools_cross sys/busybox
	xx/xx build -f -s xx_tools_cross sys/oksh

bootstrap:
	# cross-compiling compiler and libc from the host system and then
	# rebuilding them in environment isolated from the host system
	xx/xx build set/bootstrap.xx
	rm -rf /tmp/xx/

	# todo: run if prev step ok
	# creating environment only with packages build in isolation from the
	# host system
	xx/xx build set/bootstrap-base.xx
	mv /tmp/xx/bootstrap /tmp/xx/base

	# todo: run if prev step ok
	# final build of all base packages
	xx/xx build set/base.xx
	rm -rf /tmp/xx/

bootstrap_rebuild:
	xx/xx build -f set/bootstrap.xx
	rm -rf /tmp/xx/

	# todo: run if prev step ok
	xx/xx build -f set/bootstrap-base.xx
	mv /tmp/xx/bootstrap /tmp/xx/base

	# todo: run if prev step ok
	xx/xx build -f set/base.xx
	rm -rf /tmp/xx/

bootstrap_musl:
	xx/xx b set/init_musl.xx
	mv /tmp/xx/init_musl/ /tmp/xx/musl
	rm -r /tmp/xx/musl/{cross_tools,tools,usr}
	xx/xx b set/musl.xx
	rm -rf /tmp/xx/

base:
	xx/xx build set/base.xx

all:
	xx/xx build set/dev.xx
	xx/xx build set/lxc.xx
	xx/xx build set/media_cd.xx
	xx/xx build set/media_gfx.xx
	xx/xx build set/media_sdl.xx
	xx/xx build set/media_snapcast.xx
	xx/xx build set/media_text.xx
	xx/xx build set/media_video.xx
	xx/xx build set/misc.xx
	xx/xx build set/net.xx
	xx/xx build set/net_w3m.xx
	xx/xx build set/qemu.xx
	xx/xx build set/sys.xx
	xx/xx build set/x11.xx

clean_tmp:
	tools/busybox sh -c 'chmod -fR +w /tmp/xx/ || :'
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
