package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path"
	fp "path/filepath"
	"sort"
	"strings"
	"unicode"
)

func (r *runT) actionInst(world map[string]worldT, pkgs []pkgT, pkgCfgs []pkgCfgT) {

	switch {

	// todo: move to argsCheck
	case !fileExists(r.rootDir):
		errExit(errors.New(""), "target dir doesn't exist")

	// todo: move to argsCheck
	case r.toInstPerms && r.sysCfgDir == "":
		errExit(errors.New(""), "please provide a system config dir")
	case r.toInstSysCfg && r.sysCfgDir == "":
		errExit(errors.New(""), "please provide a system config dir")

	case r.toInstPerms:
		fmt.Println("* setting system permissions...")
		setSysPerm(r.rootDir)

	case r.toInstSysCfg:
		fmt.Println("* installing config files...")
		r.instSysCfg(world)
		fmt.Println("\n* setting system permissions...")
		setSysPerm(r.rootDir)

	default:
		r.checkPkgAvail(pkgs, pkgCfgs)
		r.instDefPkgs(world, pkgs, pkgCfgs)
		fmt.Println("\n* installation complete.")
	}
}

func (r *runT) instDefPkgs(world map[string]worldT, pkgs []pkgT, pkgCfgs []pkgCfgT) {
	for i, pkg := range pkgs {
		var cntInfo string
		pkgC := pkgCfgs[i]
		if pkgC.cnt {
			cntInfo = " (cnt)"
		}
		fmt.Printf("+ %-32s %s\n", pkg.name+cntInfo, pkg.setVerRel)

		loc := "/"
		if pkgC.cnt {
			loc = pkgC.cntProg
		}

		deps := r.getAllDeps(pkg, pkgC.allRunDeps, []pkgT{},
			"all", 1)
		sort.Slice(deps, func(i, j int) bool {
			return deps[i].name <= deps[j].name
		})

		for _, dep := range deps {
			fmt.Printf("%-34s %s\n", dep.name, dep.setVerRel)
			if !r.worldPkgExists(world, dep, pkgC) && !pkgC.force {
				depCfgFiles := r.getPkgCfgFiles(dep)

				instPkg(dep, pkgC, r.rootDir)
				instPkgCfg(depCfgFiles, pkgC.instDir, r.verbose)

				addPkgToWorldT(world, dep, loc)
			}
		}

		fmt.Printf("%-34s %s\n", pkg.name, pkg.setVerRel)
		if !r.worldPkgExists(world, pkg, pkgC) && !pkgC.force {
			instPkg(pkg, pkgC, r.rootDir)
			instPkgCfg(pkgC.cfgFiles, pkgC.instDir, r.verbose)

			addPkgToWorldT(world, pkg, loc)
		}
		fmt.Println()
	}

	fmt.Println("* setting system permissions...")
	setSysPerm(r.rootDir)
}

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

func pkgExists(pkg pkgT, pkgs []pkgT) bool {
	for _, p := range pkgs {
		if p.name == pkg.name && p.set == pkg.set {
			return true
		}
	}
	return false
}

func pkgCntExists(pkg pkgT, pkgCfg pkgCfgT, pkgs []pkgT,
	pkgCfgs []pkgCfgT) bool {

	for i, p := range pkgs {
		if p.name == pkg.name && p.set == pkg.set &&
			pkgCfg.cnt == pkgCfgs[i].cnt &&
			pkgCfg.cntPkg == pkgCfgs[i].cntPkg {

			return true
		}
	}
	return false
}

// return a list of files and map of sha256 hashes for each file in the pkg
func getPkgFiles(pkg pkgT) ([]string, map[string]string) {
	var files []string
	fileHash := make(map[string]string)

	shaLog := fp.Join(pkg.progDir, "log", pkg.setVerRel, "sha256.log")
	if !fileExists(shaLog) {
		return files, fileHash
	}

	fd, err := os.Open(shaLog)
	errExit(err, "can't open file: "+fd.Name())

	input := bufio.NewScanner(fd)
	for input.Scan() {
		split := strings.Split(input.Text(), "\t")
		file := split[1]
		hash := split[0]

		fileHash[file] = hash
		files = append(files, file)
	}
	fd.Close()

	return files, fileHash
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

func (r *runT) instSysCfg(world map[string]worldT) {
	r.instTargetCfg(r.rootDir, world["/"].pkgs)

	cntDir := fp.Join(r.rootDir, "/cnt/rootfs/")
	cntList := getCntList(cntDir)
	for _, cntName := range cntList {
		r.instTargetCfg("/cnt/rootfs/"+cntName, world[cntName].pkgs)
	}
}

func (r *runT) instTargetCfg(instDir string, worldPkgs map[pkgT]bool) {
	for pkg, _ := range worldPkgs {
		fmt.Printf("+ %-32s %s\n", pkg.name, pkg.set)

		// install configs
		cfgFiles := r.getPkgCfgFiles(pkg)
		instPkgCfg(cfgFiles, instDir, r.verbose)
	}
}
