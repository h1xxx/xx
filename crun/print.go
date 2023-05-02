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
