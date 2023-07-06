package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"

	fp "path/filepath"
	str "strings"
)

func (r *runT) actionInst() {
	switch {
	case r.toInstPerms:
		fmt.Println("* setting system permissions...")
		r.setSysPerm()

	case r.toInstSysCfg:
		fmt.Println("* installing config files...")
		r.instSysCfg()
		fmt.Println("\n* setting system permissions...")
		r.setSysPerm()

	default:
		r.checkPkgAvail(r.pkgs, r.pkgCfgs)
		r.createRootDirs()
		r.instDefPkgs(r.pkgs, r.pkgCfgs)
		fmt.Println("\n* installation complete.")
	}
}

func (r *runT) instDefPkgs(pkgs []pkgT, pkgCfgs []pkgCfgT) {
	for i, p := range pkgs {
		var cntInfo string
		pc := pkgCfgs[i]
		if pc.cnt {
			cntInfo = " (cnt)"
		}
		fmt.Printf("+ %-32s %s\n", p.name+cntInfo, p.setVerRel)

		worldLoc := "/"
		if pc.cnt {
			worldLoc = pc.cntProg
		}

		for _, dep := range pc.allRunDeps {
			fmt.Printf("%-34s %s\n", dep.name, dep.setVerRel)
			if !r.worldPkgExists(dep, pc) && !pc.force {
				depCfg := pc
				depCfg.cfgFiles = r.getPkgCfgFiles(dep)

				r.instPkg(dep, depCfg, worldLoc)
			}
		}

		fmt.Printf("%-34s %s\n", p.name, p.setVerRel)
		if !r.worldPkgExists(p, pc) && !pc.force {
			r.instPkg(p, pc, worldLoc)
		}
		fmt.Println()
	}

	fmt.Println("* setting system permissions...")
	r.setSysPerm()
}

// return a list of files and map of sha256 hashes for each file in the pkg
func getPkgFiles(p pkgT) ([]string, map[string]string) {
	var files []string
	fileHash := make(map[string]string)

	shaLog := fp.Join(p.progDir, "log", p.setVerRel, "sha256.log")
	if !fileExists(shaLog) {
		return files, fileHash
	}

	fd, err := os.Open(shaLog)
	errExit(err, "can't open file: "+fd.Name())

	input := bufio.NewScanner(fd)
	for input.Scan() {
		hash, file, _ := str.Cut(input.Text(), "\t")

		fileHash[file] = hash
		files = append(files, file)
	}
	fd.Close()

	return files, fileHash
}

func (r *runT) instPkg(p pkgT, pc pkgCfgT, worldLoc string) {
	fmt.Printf("  installing...\n")

	// install default system files
	Cp("/home/xx/cfg/sys/*", pc.instDir+"/")

	// don't install dummy pkg creating temporary links during bootstrap
	if str.HasSuffix(p.set, "_init") && pc.src.srcType == "files" {
		return
	}

	Cp(p.pkgDir+"/*", pc.instDir+"/")

	if !pc.muslBuild {
		createBinLinks(p.pkgDir, pc.instDir)
	}

	instPkgCfg(pc)

	addPkgToWorldDir(p, pc.instDir)
	r.addPkgToWorldT(p, worldLoc)
}

// installs config files for the pkg
func instPkgCfg(pc pkgCfgT) {
	var files []string
	for file := range pc.cfgFiles {
		files = append(files, file)
	}
	sort.Strings(files)

	for _, file := range files {
		src := pc.cfgFiles[file]
		Cp(src, fp.Join(pc.instDir, file))
	}
}

func (r *runT) instSysCfg() {
	r.instTargetCfg(r.rootDir, r.world["/"].pkgs)

	cntDir := fp.Join(r.rootDir, "/cnt/rootfs/")
	cntList := getCntList(cntDir)
	for _, cntName := range cntList {
		r.instTargetCfg("/cnt/rootfs/"+cntName, r.world[cntName].pkgs)
	}
}

func (r *runT) instTargetCfg(instDir string, worldPkgs map[pkgT]bool) {
	for p, _ := range worldPkgs {
		fmt.Printf("+ %-32s %s\n", p.name, p.set)

		var pc pkgCfgT
		pc.cfgFiles = r.getPkgCfgFiles(p)
		pc.instDir = instDir

		instPkgCfg(pc)
	}
}
