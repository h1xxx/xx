package main

import (
	"bufio"
	"errors"
	"os"
	"path"
	"sort"
	"strings"
	"unicode"

	fp "path/filepath"
)

// returns a list of dependencies
// depType possible vaules: "all", "run", "lib", "build"
func (r *runT) getAllDeps(pkg pkgT, deps, res []pkgT, depType string, depth int) []pkgT {
	if pkgExists(pkg, res) && len(deps) == 0 {
		return res
	}

	for _, dep := range deps {
		if pkgExists(dep, res) {
			continue
		}

		res = append(res, dep)

		var depDeps []pkgT
		depC := r.getPkgCfg(dep, "")
		switch depType {
		case "all":
			depDeps = depC.allRunDeps
		case "run":
			depDeps = depC.runDeps
		case "lib":
			depDeps = depC.libDeps
		case "build":
			depDeps = depC.buildDeps
		}

		res = r.getAllDeps(dep, depDeps, res, depType, depth+1)
	}

	return res
}

func (r *runT) checkPkgAvail(pkgs []pkgT, pkgCfgs []pkgCfgT) {
	for i, pkg := range pkgs {
		pkgCheck(pkg)

		pkgC := pkgCfgs[i]
		deps := r.getAllDeps(pkg, pkgC.allRunDeps, []pkgT{},
			"all", 1)
		for _, dep := range deps {
			pkgCheck(dep)
		}
	}
}

func pkgCheck(pkg pkgT) {
	if !fileExists(pkg.pkgDir) || dirIsEmpty(pkg.pkgDir) {
		msg := "package not built: " + pkg.name + " " + pkg.setVerRel
		errExit(errors.New(""), msg)
	}
}

// return a map of pkgs and list of lib names, and a list of packages
// containing needed shared libraries
func (r *runT) getPkgLibDeps(pkg pkgT) (map[pkgT][]string, []pkgT) {
	var deps []pkgT
	depLibs := make(map[pkgT][]string)
	file := fp.Join(pkg.progDir, "log", pkg.setVerRel, "shared_libs")

	if !fileExists(file) {
		return depLibs, deps
	}

	fd, err := os.Open(file)
	errExit(err, "can't open shared libs file: "+file)
	defer fd.Close()

	input := bufio.NewScanner(fd)
	for input.Scan() {
		fields := strings.Split(input.Text(), "\t")
		if len(fields) <= 1 {
			continue
		}
		if len(fields) != 5 {
			errExit(errors.New(""), "malformed file: "+file)
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
		if !pkgExists(dep, deps) && pkg.name != dep.name {
			deps = append(deps, dep)
		}
		depLibs[dep] = append(depLibs[dep], libName)
	}

	sort.Slice(deps, func(i, j int) bool {
		return deps[i].name <= deps[j].name
	})

	return depLibs, deps
}

func pkgHasLib(pkg pkgT, libName string) bool {
	// todo: if package is somewhere in world, lookup in there from a map
	file := fp.Join(pkg.progDir, "log", pkg.setVerRel, "sha256.log")
	if !fileExists(file) {
		return false
	}

	fd, err := os.Open(file)
	errExit(err, "can't open sha256 file: "+file)
	defer fd.Close()

	input := bufio.NewScanner(fd)
	for input.Scan() {
		fields := strings.Split(input.Text(), "\t")
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

	re := getRegexes()

	fd, err := os.Open(file)
	errExit(err, "can't open file: "+file)
	defer fd.Close()

	input := bufio.NewScanner(fd)
	for input.Scan() {
		line := input.Text()

		switch {
		case line == "" || rune(line[0]) == '#' || rune(line[1]) == '#':
			continue

		case unicode.IsLetter(rune(line[0])):
			currentPkg, _ = r.parseSetLine(line, re)
			deps[currentPkg] = []pkgT{}

		case rune(line[0]) == '\t':
			line = strings.Trim(line, "\t")
			dep, _ := r.parseSetLine(line, re)
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
