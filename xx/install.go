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
		r.instDefPkgs(r.pkgs, r.pkgCfgs)
		fmt.Println("\n* installation complete.")
	}
}

func (r *runT) instDefPkgs(pkgs []pkgT, pkgCfgs []pkgCfgT) {
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

		deps := r.getAllDeps(pkg, pkgC.allRunDeps, []pkgT{}, "all", 1)
		sort.Slice(deps, func(i, j int) bool {
			return deps[i].name <= deps[j].name
		})

		for _, dep := range deps {
			fmt.Printf("%-34s %s\n", dep.name, dep.setVerRel)
			if !r.worldPkgExists(dep, pkgC) && !pkgC.force {
				depCfgFiles := r.getPkgCfgFiles(dep)

				instPkg(dep, pkgC, r.rootDir)
				instPkgCfg(depCfgFiles, pkgC.instDir)

				r.addPkgToWorldT(dep, loc)
			}
		}

		fmt.Printf("%-34s %s\n", pkg.name, pkg.setVerRel)
		if !r.worldPkgExists(pkg, pkgC) && !pkgC.force {
			instPkg(pkg, pkgC, r.rootDir)
			instPkgCfg(pkgC.cfgFiles, pkgC.instDir)

			r.addPkgToWorldT(pkg, loc)
		}
		fmt.Println()
	}

	fmt.Println("* setting system permissions...")
	r.setSysPerm()
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
		split := str.Split(input.Text(), "\t")
		file := split[1]
		hash := split[0]

		fileHash[file] = hash
		files = append(files, file)
	}
	fd.Close()

	return files, fileHash
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
	for pkg, _ := range worldPkgs {
		fmt.Printf("+ %-32s %s\n", pkg.name, pkg.set)

		// install configs
		cfgFiles := r.getPkgCfgFiles(pkg)
		instPkgCfg(cfgFiles, instDir)
	}
}
