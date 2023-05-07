package main

import (
	"fmt"
	"os"

	fp "path/filepath"
)

func (r *runT) getWorld(pkgCfgs []pkgCfgT) {
	r.initWorldEntry("/")
	worldPkgs := r.getWorldPkgs(r.rootDir)
	if r.action == "build" && !r.baseEnv {
		basePkgs := r.getWorldPkgs(r.baseDir)
		worldPkgs = append(worldPkgs, basePkgs...)
	}

	for _, pkg := range worldPkgs {
		r.addPkgToWorldT(pkg, "/")
	}

	cntDir := fp.Join(r.rootDir, "/cnt/rootfs")
	cntList := getCntList(cntDir)
	for _, pkgC := range pkgCfgs {
		if pkgC.cnt && !stringExists(pkgC.cntProg, cntList) {
			cntList = append(cntList, pkgC.cntProg)
		}
	}

	for _, cntProg := range cntList {
		r.initWorldEntry(cntProg)
	}

	for _, cntProg := range cntList {
		cntRootDir := fp.Join(r.rootDir, "/cnt/rootfs/", cntProg)
		cntWorldPkgs := r.getWorldPkgs(cntRootDir)

		for _, pkg := range cntWorldPkgs {
			r.addPkgToWorldT(pkg, cntProg)
		}
	}
}

func (r *runT) initWorldEntry(entry string) {
	r.world[entry] = worldT{
		files:    make(map[string]pkgT),
		fileHash: make(map[string]string),
		pkgFiles: make(map[pkgT][]string),
		pkgs:     make(map[pkgT]bool),
	}
}

func addPkgToWorldDir(pkg pkgT, instDir string) {
	worldDir := fp.Join(instDir, "/var/xx")

	// todo: add remote host handling

	dest := fp.Join(worldDir, pkg.name, pkg.setVerRel)
	err := os.MkdirAll(dest, 0700)
	errExit(err, "couldn't create "+dest)

	src := fp.Join(pkg.progDir, "log", pkg.setVerRel, "sha256.log")
	if fileExists(src) {
		Cp(src, dest)
	}

	src = fp.Join(pkg.progDir, "log", pkg.setVerRel, "shared_libs")
	if fileExists(src) {
		Cp(src, dest)
	}
}

func (r *runT) addPkgToWorldT(pkg pkgT, loc string) {
	// todo: read file from world dir not from /home/xx
	files, fileHash := getPkgFiles(pkg)
	for file, hash := range fileHash {
		r.world[loc].files[file] = pkg
		r.world[loc].fileHash[file] = hash
	}

	r.world[loc].pkgFiles[pkg] = files
	r.world[loc].pkgs[pkg] = true
}

func (r *runT) worldPkgExists(pkg pkgT, pkgC pkgCfgT) bool {
	// todo: try to simplify this by making instDir == rootDir
	worldDir := fp.Join(r.rootDir, "/var/xx")
	if pkgC.cnt {
		worldDir = fp.Join(pkgC.instDir, "/var/xx")
	}
	// todo: add remote host handling
	f := fp.Join(worldDir, pkg.name, pkg.setVerRel)

	pkgInWorldDir := fileExists(f)
	loc := "/"
	if pkgC.cnt {
		loc = pkgC.cntProg
	}
	_, pkgInWorldT := r.world[loc].pkgs[pkg]

	if pkgInWorldDir && !pkgInWorldT {
		fmt.Println(pkg.name, pkg.setVerRel, "not consistent in the world")
		fmt.Println("in world dir:", pkgInWorldDir, f)
		fmt.Println("in world var, loc, len(deps):", pkgInWorldT, loc,
			len(r.world[loc].pkgs))
	}

	return pkgInWorldDir && pkgInWorldT
}
