package main

import "fmt"

func (r *runT) printRunParams() {
	prStr("install root dir", r.rootDir)
	prStr("config dir", r.sysCfgDir)
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
	br()

	prBool("show diff to prev. build", r.diffBuild)
	prInt("diff timeframe", int(r.diffHours))
	br()

	prBool("show info on deps", r.infoDeps)
	prBool("check system integryity", r.infoInteg)
	br()

	prInt("count of pkgs", len(r.pkgs))
	prInt("count of world pkgs", len(r.world["/"].pkgs))
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

func prHigh(formatS string, a ...any) {
	fmt.Printf("\033[01m* "+formatS+"\033[00m\n", a...)
}

func prDebug(formatS string, a ...any) {
	fmt.Printf("debug: "+formatS+"\n", a...)
}

func br() {
	fmt.Println()
}

func printUsage() {
	fmt.Println(`usage: xx <action> [parameters] <xx set file or a program>

actions/parameters:

(b)uild         build packages
-f              force build of all packages
-s              build set to build for a single package (default: std)
-V              version to build for a single package

(i)nstall       install already built packages
-f              force installation of all packages
-r              root dir to install packages to
-c              system config dir
-P              only set system permissions
-C              only install configs from the system config dir

(s)ource        download and verify source files

(d)iff          show diff between pkg builds
-h              time horizon in hours to search for diffs (default: 24)
-b              show diff against previous build (default: previous version)

i(n)fo          show additional information on xx system
-a              information on all package, incl. dependencies
-i              verify system integrity

(u)pdate        update ini files
-i              get information on latest packages available
`)
}
