package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"

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

	// if base environment is built protect it from further changes
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
		} else if r.isSepSys && !pkgC.cnt {
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

func (r *runT) createPkg(pkg pkgT, pkgC pkgCfgT) pkgT {
	if pkg.newRel != "00" && !pkgC.force || pkgC.src.srcType == "files" {
		return pkg
	}

	createPkgDirs(pkg, pkgC)
	r.getSrc(pkg, pkgC)

	r.execStep("prepare", pkg, pkgC)
	r.execStep("configure", pkg, pkgC)
	r.execStep("build", pkg, pkgC)

	err := os.MkdirAll(pkg.newPkgDir, 0700)
	errExit(err, "couldn't create dir: "+pkg.newPkgDir)
	r.execStep("pkg_create", pkg, pkgC)

	r.pkgBuildCheck(pkg, pkgC)
	moveLogs(pkg, pkgC)
	r.saveHelp(pkg, pkgC)
	cleanup(pkg, pkgC)
	dumpSHA256(pkg)

	if dirIsEmpty(pkg.newPkgDir) {
		fmt.Println("! pkg empty:", pkg.newPkgDir)
	}

	for _, s := range pkgC.steps.subPkgs {
		subPkg := getSubPkg(pkg, s.suffix)
		fmt.Printf("  creating subpkg %s...\n", subPkg.set)
		createSubPkg(pkg, subPkg, s.files)
	}

	// remove old pkg from world
	delete(r.world["/"].pkgs, pkg)
	for _, s := range pkgC.steps.subPkgs {
		subPkg := getSubPkg(pkg, s.suffix)
		delete(r.world["/"].pkgs, subPkg)
	}

	// get new release info after the build
	pkg.setVerRel = ""
	pkg.rel, pkg.prevRel, pkg.newRel = getPkgRels(pkg)
	pkg = getPkgSetVers(pkg)
	pkg = getPkgDirs(pkg)

	// add a new pkg and all subpkgs to root of the world;
	// no cnt here as only build step executes this
	r.addPkgToWorldT(pkg, "/")
	for _, s := range pkgC.steps.subPkgs {
		subPkg := getSubPkg(pkg, s.suffix)
		r.addPkgToWorldT(subPkg, "/")
	}

	// dump shared libs for main pkg and for subpkgs
	if !pkgC.crossBuild {
		r.dumpSharedLibs(pkg)
	}
	for _, s := range pkgC.steps.subPkgs {
		subPkg := getSubPkg(pkg, s.suffix)
		r.dumpSharedLibs(subPkg)
		r.selfLibsExist(subPkg)
	}

	return pkg
}

func (r *runT) pkgBuildCheck(pkg pkgT, pkgC pkgCfgT) {
	var noNoDirs []string
	var noNoLibRe *regexp.Regexp

	re := getRegexes()

	muslNoNo := []string{"/usr", "/lib64", "/local", "/opt"}
	glibcNoNo := []string{"/bin", "/sbin", "/lib", "/lib64", "/opt",
		"/include", "/share", "/usr/lib64", "/usr/local"}

	switch {
	case pkgC.crossBuild && pkgC.muslBuild:
		return
	case pkgC.muslBuild:
		noNoDirs = muslNoNo
		noNoLibRe = re.noNoSharedLib
	default:
		noNoDirs = glibcNoNo
		noNoLibRe = re.noNoStaticLib
	}

	dirs, err := walkDir(pkg.newPkgDir, "dirs")
	errExit(err, "can't read pkg dirs in "+pkg.newPkgDir)

	for _, dir := range dirs {
		dir = strings.TrimPrefix(dir, pkg.newPkgDir)
		msg := "WARNING! incorrect dir: %s\n"
		for _, noNo := range noNoDirs {
			if strings.HasPrefix(dir, noNo) {
				fmt.Printf(msg, dir)
			}
		}
	}

	files, err := walkDir(pkg.newPkgDir, "files")
	errExit(err, "can't read pkg dirs in "+pkg.newPkgDir)
	for _, file := range files {
		f := strings.TrimPrefix(file, pkg.newPkgDir)
		msg := "WARNING! incorrect lib: %s\n"

		isNoNoLib := noNoLibRe.MatchString(f)
		if isNoNoLib {
			fmt.Printf(msg, f)
		}

		isStaticBin := re.staticBin.MatchString(f)
		if isStaticBin && binHasInterpreter(file) {
			msg := "WARNING! non static bin: %s\n"
			fmt.Printf(msg, f)
		}

		isGlibcBin := re.glibcBin.MatchString(f)
		if isGlibcBin && binHasWeirdInterpreter(file) {
			msg := "WARNING! incorrect interpreter: %s\n"
			fmt.Printf(msg, f)
		}
	}
}

func binHasInterpreter(file string) bool {
	binDir := "/home/xx/bin"
	c := fmt.Sprintf("%s/file -m %s/magic.mgc %s", binDir, binDir, file)

	cmd := exec.Command(binDir+"/bash", "-c", c)
	out, err := cmd.CombinedOutput()
	errExit(err, "can't run 'file' binary from xx tools\n"+string(out))

	if strings.Contains(string(out), "interpreter /") {
		return true
	}

	return false
}

func binHasWeirdInterpreter(file string) bool {
	binDir := "/home/xx/bin"
	c := fmt.Sprintf("%s/file -m %s/magic.mgc %s", binDir, binDir, file)

	cmd := exec.Command(binDir+"/bash", "-c", c)
	out, err := cmd.CombinedOutput()
	errExit(err, "can't run 'file' binary from xx tools")

	if strings.Contains(string(out), "interpreter /lib") ||
		strings.Contains(string(out), "interpreter /usr/lib64") {
		return true
	}

	return false
}

func getSubPkg(pkg pkgT, suffix string) pkgT {
	subPkg := pkg
	subPkg.set = pkg.set + "_" + suffix
	subPkg = getPkgSetVers(subPkg)
	subPkg = getPkgDirs(subPkg)

	return subPkg
}

func createSubPkg(pkg, subPkg pkgT, files []string) {
	for _, f := range files {
		src := fp.Join(pkg.newPkgDir, f)
		dest := fp.Join(subPkg.newPkgDir, f)
		Mv(src, dest)
		RemEmptyDirs(fp.Dir(src))
		MoveShaInfo(pkg, subPkg, f)
	}
}

func MoveShaInfo(pkg, subPkg pkgT, file string) {
	src := fp.Join(pkg.progDir, "log", pkg.setVerNewRel, "sha256.log")
	dest := fp.Join(subPkg.progDir, "log", subPkg.setVerNewRel, "sha256.log")
	file = strings.Replace(file, "*", ".*", -1)

	err := os.MkdirAll(fp.Dir(dest), 0750)
	errExit(err, "can't create dest dir: "+fp.Dir(dest))

	bb := "/home/xx/bin/busybox"
	c := bb + " grep \t" + file + " " + src + " > " + dest

	cmd := exec.Command(bb, "sh", "-c", c)
	out, err := cmd.CombinedOutput()

	errExit(err, "can't copy sha lines from "+src+" to "+dest+
		"\n"+string(out)+"\n"+strings.Join(cmd.Args, " "))

	c = bb + " sed -i '\\|\t" + file + "|d' " + src

	cmd = exec.Command(bb, "sh", "-c", c)
	out, err = cmd.CombinedOutput()

	errExit(err, "can't remove sha lines from "+src+
		"\n"+string(out)+"\n"+strings.Join(cmd.Args, " "))
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
