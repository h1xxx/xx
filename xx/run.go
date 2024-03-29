package main

import (
	fp "path/filepath"
	str "strings"
)

func (r *runT) getRunVars() {
	r.getBuildEnv()
	r.getRootDir()
	r.getBaseInfo()
	r.detectInit()

	r.date = getCurrentDate()
	r.re = getRegexes()

	r.runDeps = r.readDeps("run")
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
		errExit(ERR, "can't find build env in", r.actionTarget)
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

	r.baseLinked = fileExists(fp.Join(r.rootDir, "base_linked"))
	r.baseOk = fileExists(fp.Join(r.baseDir, "base_ok"))
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

func (r *runT) detectSeparateSys() {
	r.isSepSys = r.pkgs[0].categ == "alpine"
}
