package main

import "fmt"

func (r *runT) printRunParams() {
	prStr("install root dir", r.rootDir)
	prStr("config dir", r.sysCfgDir)
	prStr("set file", r.setFileName)
	prStr("build environment", r.buildEnv)
	prStr("base dir", r.baseDir)
	prStr("base set file", r.baseFile)
	prBool("is base environment", r.baseEnv)
	prBool("is musl environment", r.muslEnv)
	prBool("is bootstrap phase", r.isInit)
	prStr("action target", r.actionTarget)
	prStr("action", r.action)
	prStr("current date", r.date)
	br()

	prBool("force all", r.forceAll)
	prStr("fixed pkg set", r.fixedSet)
	prStr("fixed pkg ver", r.fixedVer)
	br()

	prBool("only install perms", r.toInstPerms)
	prBool("only install sys config", r.toInstSysCfg)
	prBool("verbose", r.verbose)
	br()

	prBool("show diff to prev. build", r.diffBuild)
	prInt("diff timeframe", int(r.diffHours))
	br()

	prBool("show info on deps", r.infoDeps)
	prBool("check system integryity", r.infoInteg)
	br()

	prInt("count of build deps", len(r.buildDeps))
	prInt("count of run deps", len(r.runDeps))
	br()
}

func prStr(title, s string) {
	fmt.Printf("%-24s%s\n", title, s)
}

func prInt(title string, i int) {
	fmt.Printf("%-24s%d\n", title, i)
}

func prBool(title string, b bool) {
	fmt.Printf("%-24s%v\n", title, b)
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
