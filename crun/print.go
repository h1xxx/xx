package main

import (
	"fmt"

	str "strings"
)

func (r *runT) printDebug() {
	prStr("container", r.cnt)
	prStr("program", r.bin)
	prStr("program args", str.Join(r.args, " "))
	br()
	prStr("config file", r.cntCfgFile)
	br()
	prInt("container id", r.cntId)
	prStr("config", r.cntConfStr)
	prStr("ip address", r.cntIP)
	prBool("net enable", r.cntConf.net)
	br()
	prStr("cnt dir", r.dirs.cnt)
	prStr("home dir", r.dirs.home)
	prStr("bind dir", r.dirs.bind)
	br()
	prStr("bind targets:", "")
	for _, l := range r.bindTargets {
		fmt.Println("\t", l)
	}
	br()
}

func prStr(title, s string) {
	fmt.Printf("%-20s%s\n", title, s)
}

func prInt(title string, i int) {
	fmt.Printf("%-20s%d\n", title, i)
}

func prBool(title string, b bool) {
	fmt.Printf("%-20s%v\n", title, b)
}

func pr(formatS string, a ...any) {
	fmt.Printf(formatS+"\n", a...)
}

func prD(formatS string, a ...any) {
	fmt.Printf("debug: "+formatS+"\n", a...)
}

func br() {
	fmt.Println()
}

func printHelp() {
	fmt.Println(`usage: crun <program> [crun args] [program args]
usage: <link to crun named after program> [crun args] [program args]

crun args:

+b, ++bind <path>   - bind mount a path to the container work dir
+c, ++config <file> - path to a cnt config file (default: /etc/cnt.conf)
+g, ++get <dest>    - copy all files from container work dir
+s, ++shell         - start a shell in the container
+n, ++no-config     - don't write a new lxc config file (only with ++shell)
+d, ++debug         - print info on crun variables and execution
+h, ++help          - this help


usage: crun [crun args]

-l, --links         - recreate links to crun in /cnt/bin based on cnt config
-c, --config <file> - path to a cnt config file (default: /etc/cnt.conf)
-d, --debug         - print info on crun variables and execution
-h, --help          - this help

`)
}
