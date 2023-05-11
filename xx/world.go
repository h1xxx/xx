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

	for _, p := range worldPkgs {
		r.addPkgToWorldT(p, "/")
	}

	cntDir := fp.Join(r.rootDir, "/cnt/rootfs")
	cntList := getCntList(cntDir)
	for _, pc := range pkgCfgs {
		if pc.cnt && !stringExists(pc.cntProg, cntList) {
			cntList = append(cntList, pc.cntProg)
		}
	}

	for _, cntProg := range cntList {
		r.initWorldEntry(cntProg)
	}

	for _, cntProg := range cntList {
		cntRootDir := fp.Join(r.rootDir, "/cnt/rootfs/", cntProg)
		cntWorldPkgs := r.getWorldPkgs(cntRootDir)

		for _, p := range cntWorldPkgs {
			r.addPkgToWorldT(p, cntProg)
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

func addPkgToWorldDir(p pkgT, instDir string) {
	worldDir := fp.Join(instDir, "/var/xx")

	// todo: add remote host handling

	dest := fp.Join(worldDir, p.name, p.setVerRel)
	err := os.MkdirAll(dest, 0700)
	errExit(err, "couldn't create "+dest)

	src := fp.Join(p.progDir, "log", p.setVerRel, "sha256.log")
	if fileExists(src) {
		Cp(src, dest)
	}

	src = fp.Join(p.progDir, "log", p.setVerRel, "shared_libs")
	if fileExists(src) {
		Cp(src, dest)
	}
}

func (r *runT) addPkgToWorldT(p pkgT, loc string) {
	// todo: read file from world dir not from /home/xx
	files, fileHash := getPkgFiles(p)
	for file, hash := range fileHash {
		r.world[loc].files[file] = p
		r.world[loc].fileHash[file] = hash
	}

	r.world[loc].pkgFiles[p] = files
	r.world[loc].pkgs[p] = true
}

func (r *runT) worldPkgExists(p pkgT, pc pkgCfgT) bool {
	// todo: try to simplify this by making instDir == rootDir
	worldDir := fp.Join(r.rootDir, "/var/xx")
	if pc.cnt {
		worldDir = fp.Join(pc.instDir, "/var/xx")
	}
	// todo: add remote host handling
	f := fp.Join(worldDir, p.name, p.setVerRel)

	pkgInWorldDir := fileExists(f)
	loc := "/"
	if pc.cnt {
		loc = pc.cntProg
	}
	_, pkgInWorldT := r.world[loc].pkgs[p]

	if pkgInWorldDir && !pkgInWorldT {
		fmt.Println(p.name, p.setVerRel, "not consistent in the world")
		fmt.Println("in world dir:", pkgInWorldDir, f)
		fmt.Println("in world var, loc, len(deps):", pkgInWorldT, loc,
			len(r.world[loc].pkgs))
	}

	return pkgInWorldDir && pkgInWorldT
}
