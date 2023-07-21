package main

var cfgTemplate = `lxc.uts.name = cnt-%s
lxc.arch = x86_64

lxc.idmap = u 0 100000 1
lxc.idmap = u 1002 1002 1

lxc.idmap = g 0 100000 1
lxc.idmap = g 5 100005 1
lxc.idmap = g 6 6 1
lxc.idmap = g 31 31 4
lxc.idmap = g 1002 1002 1

lxc.init.uid = 1002
lxc.init.gid = 1002

lxc.tty.max = 1
lxc.pty.max = 16

lc.signal.halt = SIGUSR1
lxc.signal.reboot = SIGTERM

lxc.rootfs.path = dir:%s
lxc.mount.auto = cgroup:ro proc:rw sys:ro
lxc.autodev = 1

lxc.environment = TERM=screen
lxc.environment = HOME=/home/cnt
lxc.environment = PATH=/bin:/sbin
lxc.environment = BASH_ENV=/home/cnt/.profile
lxc.environment = LC_ALL=en_US.utf8

lxc.mount.entry = /dev/tty dev/tty none bind,create=file 0 0
`

var cfgNet = `
lxc.net.0.type = empty
lxc.net.0.type = veth
lxc.net.0.veth.mode = router
lxc.net.0.ipv4.address = %s
lxc.net.0.ipv4.gateway = 10.64.64.1
lxc.net.0.flags = up
lxc.net.0.link = lxc-br0

`

var cfgNetEmpty = `
lxc.net.0.type = empty

`
