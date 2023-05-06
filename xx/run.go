package main

import (
	"fmt"
	"time"

	fp "path/filepath"
	str "strings"
)

func getRunVars() runT {
	var args argsT
	var r runT

	r.buildEnv = getBuildEnv(r.actionTarget)
	r.baseDir = "/tmp/xx/base"
	r.baseFile = "/home/xx/set/base.xx"
	if r.buildEnv == "base" || r.buildEnv == "musl_base" {
		r.baseEnv = true
	}
	if str.HasPrefix(r.buildEnv, "musl_") {
		r.muslEnv = true
		r.baseDir = "/tmp/xx/musl_base"
		r.baseFile = "/home/xx/set/musl_base.xx"
	}

	if str.HasPrefix(r.buildEnv, "init_") {
		r.isInit = true
	}

	// this file shows that the base pkgs are currently being bootstrapped
	if !r.muslEnv && fileExists("/tmp/xx/base/bootstrap") {
		r.isInit = true
	}

	r.rootDir = getRootDir(args, r.buildEnv)

	if !isPkgString(r.actionTarget) {
		r.setFileName = fp.Base(r.actionTarget)
	}

	if *args.set != "std" {
		r.fixedSet = *args.set
	}

	if *args.Ver != "latest" {
		r.fixedVer = *args.Ver
	}

	t := time.Now()
	fStr := "%d-%.2d-%.2d"
	r.date = fmt.Sprintf(fStr, t.Year(), t.Month(), t.Day())

	r.sysCfgDir = fp.Clean(*args.c)

	r.toInstPerms = *args.P
	r.toInstSysCfg = *args.C
	r.verbose = *args.v

	r.infoDeps = *args.d
	r.infoInteg = *args.i

	//if !isPkgString(r.actionTarget) {
	//	r.runDeps = r.readDeps("run")
	//	r.buildDeps = r.readDeps("build")
	//}

	return r
}

func getRootDir(args argsT, buildEnv string) string {
	var rootDir string

	if *args.rootDir == "" {
		rootDir = fp.Join("/tmp/xx/", buildEnv)
	} else if str.Contains(*args.rootDir, ":/") {
		// just clean up the path when the host is remote
		split := str.Split(*args.rootDir, ":")
		host := split[0]
		path := split[1]
		path = fp.Clean(path)
		rootDir = host + ":" + path
	} else {
		rootDir = fp.Clean(*args.rootDir)
	}

	return rootDir
}
