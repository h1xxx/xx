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
	fmt.Println(`usage: xx <action> [parameters]

actions/parameters:

(b)uild         build packages
-f, --force     force build of all packages
-s, --pkg-set   build set to build for a single package (default: std)
-v, --pkg-ver   version to build for a single package
-t, --target    build target (xx set file or a program)

(i)nstall       install already built packages
-f, --force     force installation of all packages
-r, --root-dir  root dir to install packages to
-c, --cfg-dir   system config dir
-s, --pkg-set   fixed package set to install for all packages
-v, --pkg-ver   fixed version to install for all packages
-P, --Perms     only set system permissions
-C, --Config    only install configs from the system config dir
-t, --target    install target (xx set file or a program)

(s)ource        download and verify source files
-t, --target    package target (xx set file or a program)

(d)iff          show diff between pkg builds
-h, --hours     time horizon in hours to search for diffs (default: 24)
-b, --build     show diff against previous build (default: previous version)
-s, --pkg-set   build set to build for a single package (default: std)
-v, --pkg-ver   version to build for a single package
-t, --target    diff target (xx set file or a program)

i(n)fo          show additional information on xx system
-a, --all       information on all packages, incl. dependencies
-i, --integrity verify system integrity
-t, --target    package target (xx set file or a program)

(u)pdate        update ini files
-i, --info      get information on latest packages available
-t, --target    package target (xx set file or a program)

(sh)ell         start a shell inside the build environment
-p, --pkg-name  package to configure for shell
-s, --pkg-set   fixed package set to configure for shell
-v, --pkg-ver   fixed version to configure for shell
-x, --extract   extract package source code to build dir
-i, --install   install packages from the xx set file
-t, --target    build target (xx set file or a program)
`)
}
