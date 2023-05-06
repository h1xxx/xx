package main

import (
	"fmt"

	fp "path/filepath"
	str "strings"
)

func (r *runT) getRunVars() {
	r.getBuildEnv()
	r.getBaseInfo()
	r.getRootDir()
	r.detectInit()

	r.date = getCurrentDate()

	if !r.targetIsSinglePkg {
		r.setFileName = fp.Base(r.actionTarget)
	}

	r.prDebug("getting run-time dependencies...")
	r.runDeps = r.readDeps("run")

	r.prDebug("getting build-time dependencies...")
	r.buildDeps = r.readDeps("build")

	r.world = make(map[string]worldT)
}

func (r *runT) getBuildEnv() {
	if r.targetIsSinglePkg {
		r.buildEnv = fp.Base(r.actionTarget)
		return
	}

	file := fp.Base(r.actionTarget)
	fields := str.Split(file, ".")

	r.buildEnv = str.Join(fields[:len(fields)-1], ".")

	if r.buildEnv == "init_glibc_cp" {
		r.buildEnv = "init_glibc"
	}

	if r.buildEnv == "" {
		msg := "can't find build env in %s"
		errExit(fmt.Errorf(msg, r.actionTarget), "")
	}
}

func (r *runT) getBaseInfo() {
	r.baseDir = "/tmp/xx/base"
	r.baseFile = "/home/xx/set/base.xx"

	if str.HasPrefix(r.buildEnv, "musl_") {
		r.muslEnv = true
	}

	if str.HasPrefix(r.fixedSet, "musl") {
		r.muslEnv = true
	}

	if r.muslEnv {
		r.baseDir = "/tmp/xx/musl_base"
		r.baseFile = "/home/xx/set/musl_base.xx"
	}

	if r.buildEnv == "base" || r.buildEnv == "musl_base" {
		r.baseEnv = true
	}
}

func (r *runT) getRootDir() {
	switch {
	case r.rootDir == "":
		r.rootDir = fp.Join("/tmp/xx/", r.buildEnv)

	case str.Contains(r.rootDir, ":/"):
		// clean the path component when the host is remote
		split := str.Split(r.rootDir, ":")
		host := split[0]
		path := split[1]
		path = fp.Clean(path)
		r.rootDir = host + ":" + path

	default:
		r.rootDir = fp.Clean(r.rootDir)
	}
}

func (r *runT) detectInit() {
	if str.HasPrefix(r.buildEnv, "init_") {
		r.isInit = true
	}

	// this file shows that the base pkgs are currently being bootstrapped
	if !r.muslEnv && fileExists("/tmp/xx/base/bootstrap") {
		r.isInit = true
	}
}
