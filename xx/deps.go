package main

import (
	"bufio"
	"os"
	"path"
	"sort"
	"unicode"

	fp "path/filepath"
	str "strings"
)

// returns a list of all run-time dependencies recursively
func (r *runT) getAllRunDeps(p pkgT, res *[]pkgT) {
	// todo: drop this once pkg[pkgCfg] map is implemented
	// test if this speeds anything and what's the mem consumption
	pc := r.getPkgCfg(p, "")

	for _, dep := range pc.runDeps {
		if !pkgExists(dep, *res) {
			*res = append(*res, dep)
			r.getAllRunDeps(dep, res)
		}
	}

	for _, dep := range pc.libDeps {
		if !pkgExists(dep, *res) {
			*res = append(*res, dep)
			r.getAllRunDeps(dep, res)
		}
	}

	if !pkgExists(p, *res) {
		*res = append(*res, p)
	}
}

func (r *runT) checkPkgAvail(pkgs []pkgT, pkgCfgs []pkgCfgT) {
	for i, p := range pkgs {
		pkgCheck(p)

		pc := pkgCfgs[i]
		for _, dep := range pc.allRunDeps {
			pkgCheck(dep)
		}
	}
}

func pkgCheck(p pkgT) {
	if !fileExists(p.pkgDir) || dirIsEmpty(p.pkgDir) {
		msg := "package not built: " + p.name + " " + p.setVerRel
		errExit(ERR, msg)
	}
}

// return a map of pkgs and list of lib names, and a list of packages
// containing needed shared libraries
func (r *runT) getPkgLibDeps(p pkgT) (map[pkgT][]string, []pkgT) {
	var deps []pkgT
	depLibs := make(map[pkgT][]string)
	file := fp.Join(p.progDir, "log", p.setVerRel, "shared_libs")

	if !fileExists(file) {
		return depLibs, deps
	}

	fd, err := os.Open(file)
	errExit(err, "can't open shared libs file: "+file)
	defer fd.Close()

	input := bufio.NewScanner(fd)
	for input.Scan() {
		fields := str.Split(input.Text(), "\t")
		if len(fields) <= 1 {
			continue
		}
		if len(fields) != 5 {
			errExit(ERR, "malformed file:", file)
		}
		libName := fields[0]
		name := fields[1]
		pkgSet := fields[2]
		ver := fields[3]
		if name == "" {
			continue
		}

		// todo: this logic ignores newer, but not latest versions
		dep := r.getPkg(name, pkgSet, ver)
		depLatest := r.getPkg(name, pkgSet, "latest")
		if dep != depLatest && pkgHasLib(depLatest, libName) {
			dep = depLatest
		}
		if !pkgExists(dep, deps) && p.name != dep.name {
			deps = append(deps, dep)
		}
		depLibs[dep] = append(depLibs[dep], libName)
	}

	sort.Slice(deps, func(i, j int) bool {
		return deps[i].name <= deps[j].name
	})

	return depLibs, deps
}

func pkgHasLib(p pkgT, libName string) bool {
	// todo: if package is somewhere in world, lookup in there from a map
	file := fp.Join(p.progDir, "log", p.setVerRel, "sha256.log")
	if !fileExists(file) {
		return false
	}

	fd, err := os.Open(file)
	errExit(err, "can't open sha256 file: "+file)
	defer fd.Close()

	input := bufio.NewScanner(fd)
	for input.Scan() {
		fields := str.Split(input.Text(), "\t")
		fileName := path.Base(fields[1])
		if fileName == libName {
			return true
		}
	}

	return false
}

// read dependencies from xx configuration files
// depType can be "run" or "build"
func (r *runT) readDeps(depType string) map[pkgT][]pkgT {
	var file string
	var currentPkg pkgT
	deps := make(map[pkgT][]pkgT)

	if depType == "run" {
		file = "/home/xx/set/deps_run"
	} else if depType == "build" {
		file = "/home/xx/set/deps_build"
	}

	fd, err := os.Open(file)
	errExit(err, "can't open file: "+file)
	defer fd.Close()

	input := bufio.NewScanner(fd)
	for input.Scan() {
		line := input.Text()

		switch {
		case line == "" || line[0] == '#' || line[1] == '#':
			continue

		case unicode.IsLetter(rune(line[0])):
			currentPkg, _ = r.parseSetLine(line, r.re)
			deps[currentPkg] = []pkgT{}

		case line[0] == '\t':
			line = str.Trim(line, "\t")
			dep, _ := r.parseSetLine(line, r.re)
			deps[currentPkg] = append(deps[currentPkg], dep)
		}
	}

	for pkg, depsSlice := range deps {
		sort.Slice(depsSlice, func(i, j int) bool {
			return depsSlice[i].name <= depsSlice[j].name
		})
		deps[pkg] = depsSlice
	}

	return deps
}
