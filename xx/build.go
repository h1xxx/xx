package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"sort"

	fp "path/filepath"
	str "strings"
)

func (r *runT) actionBuild() {
	r.createBuildDirs()

	// boostrap base packages and exit
	if r.isInit {
		prHigh("bootstrapping %s...", r.buildEnv)
		r.buildInstPkgs()
		return
	}

	// base env must exist first, copy it if it's not in place
	if !r.baseOk {
		prHigh("copying base dir...")
		r.installBase()
	}

	// create links in root dir to base dir
	if !r.baseLinked && !r.baseEnv && !r.isSepSys {
		linkBaseDir(r.rootDir, r.baseDir)
	}

	prHigh("processing %s...", r.buildEnv)
	r.createRootDirs()
	r.buildInstPkgs()

	if r.baseEnv {
		_, _ = os.Create(fp.Join(r.baseDir, "base_ok"))
		protectBaseDir(r.baseDir)
	}
}

func (r *runT) installBase() {
	baseRun := *r
	baseRun.rootDir = r.baseDir
	baseRun.installCnt = false

	baseRun.createRootDirs()
	pkgs, pkgCfgs := baseRun.parseBuildEnvFile(r.baseFile)

	for i, pkg := range pkgs {
		pkgC := pkgCfgs[i]

		// find the latest built pkg, don't build anything new
		if !fileExists(pkg.pkgDir) {
			pkg = getPkgPrevVer(pkg)
			pkgC = r.getPkgCfg(pkg, "")
		}
		if !fileExists(pkg.pkgDir) {
			msg := "can't find previous pkg version for %s %s"
			errExit(fmt.Errorf(msg, pkg.name, pkg.ver), "")
		}

		fmt.Printf("+ %-32s %s\n", pkg.name, pkg.setVerRel)
		r.instPkg(pkg, pkgC)
		instPkgCfg(pkgC.cfgFiles, r.baseDir)
		r.addPkgToWorldT(pkg, "/")

		// double check if shared libraries are ok
		if !r.isInit && !pkgC.muslBuild {
			r.selfLibsExist(pkg)
		}
	}

	baseOkFile := fp.Join(r.baseDir, "base_ok")
	_, err := os.Create(baseOkFile)
	errExit(err, "can't create base_ok file in "+baseOkFile)

	protectBaseDir(r.baseDir)
}

func (r *runT) buildInstPkgs() {
	for i, pkg := range r.pkgs {
		fmt.Printf("+ %-32s %s\n", pkg.name, pkg.setVerRel)
		pkgC := r.pkgCfgs[i]

		if pkgC.subPkg {
			// get the latest subpkg release in case when the main
			// pkg was rebuilt
			pkg.setVerRel = ""
			pkg.rel, pkg.prevRel, pkg.newRel = getPkgRels(pkg)
			pkg = getPkgSetVers(pkg)
			pkg = getPkgDirs(pkg)
		} else {
			pkg = r.createPkg(pkg, pkgC)
		}

		if str.HasSuffix(pkg.set, "_tools_cross") {
			continue
		} else if r.worldPkgExists(pkg, pkgC) && !pkgC.force {
			continue
		} else if pkg.categ == "alpine" && !pkgC.cnt {
			continue
		} else {
			r.instPkg(pkg, pkgC)
			instPkgCfg(pkgC.cfgFiles, pkgC.instDir)

			loc := "/"
			if pkgC.cnt {
				loc = pkgC.cntProg
			}
			r.addPkgToWorldT(pkg, loc)

			// double check if shared libraries are ok
			if !r.isInit && !pkgC.muslBuild {
				r.selfLibsExist(pkg)
			}
		}
	}
}

// checks if the built pkg contains self-referencing shared libraries;
// these are assigned by defult so a check is necessary
func (r *runT) selfLibsExist(pkg pkgT) {
	depLibs, _ := r.getPkgLibDeps(pkg)
	files := r.world["/"].pkgFiles[pkg]
	for _, lib := range depLibs[pkg] {
		var found bool
		for _, file := range files {
			fileName := path.Base(file)
			if fileName == lib {
				found = true
				break
			}
		}
		if !found {
			errExit(errors.New(""),
				"can't find shared lib assigned to pkg: "+lib)
		}
	}
}

func (r *runT) instPkg(pkg pkgT, pkgC pkgCfgT) {
	fmt.Printf("  installing...\n")

	// install default system files
	Cp("/home/xx/cfg/sys/*", pkgC.instDir+"/")

	// don't install dummy pkg creating temporary links during bootstrap
	if str.HasSuffix(pkg.set, "_init") && pkgC.src.srcType == "files" {
		return
	}

	busybox := "/home/xx/bin/busybox"
	c := busybox + " cp -rf " + pkg.pkgDir + "/* " + pkgC.instDir
	if str.Contains(pkgC.instDir, ":/") {
		c = "scp -q " + pkg.pkgDir + "/* " + pkgC.instDir
	}
	cmd := exec.Command(busybox, "sh", "-c", c)
	out, err := cmd.Output()
	errExit(err, "can't copy "+pkg.pkgDir+" to "+pkgC.instDir+
		"\n"+string(out)+"\n"+str.Join(cmd.Args, " "))

	if !pkgC.muslBuild {
		createBinLinks(pkg.pkgDir, pkgC.instDir)
	}

	// todo: move this outside
	addPkgToWorldDir(pkg, pkgC.instDir)
}

// installs config files for the pkg
func instPkgCfg(cfgFiles map[string]string, instDir string) {
	var files []string
	for file := range cfgFiles {
		files = append(files, file)
	}
	sort.Strings(files)

	for _, file := range files {
		src := cfgFiles[file]
		dest := fp.Join(instDir, file)
		Cp(src, dest)
	}
}
