package main

import (
	"bufio"
	"crypto/sha256"
	"debug/elf"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"regexp"
	"sort"

	fp "path/filepath"
	str "strings"
)

func (r *runT) actionBuild() {
	r.createBuildDirs()

	// boostrap base packages and exit
	if r.isInit {
		prHigh("bootstrapping %s...", r.buildEnv)
		r.buildPkgs()
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
	r.buildPkgs()

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
		r.instPkg(pkg, pkgC, "/")
		instPkgCfg(pkgC.cfgFiles, r.baseDir)

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

func (r *runT) buildPkgs() {
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
			r.instPkg(pkg, pkgC, "/")
			instPkgCfg(pkgC.cfgFiles, pkgC.instDir)

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
		dir = str.TrimPrefix(dir, pkg.newPkgDir)
		msg := "WARNING! incorrect dir: %s\n"
		for _, noNo := range noNoDirs {
			if str.HasPrefix(dir, noNo) {
				fmt.Printf(msg, dir)
			}
		}
	}

	files, err := walkDir(pkg.newPkgDir, "files")
	errExit(err, "can't read pkg dirs in "+pkg.newPkgDir)
	for _, file := range files {
		f := str.TrimPrefix(file, pkg.newPkgDir)
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

	if str.Contains(string(out), "interpreter /") {
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

	if str.Contains(string(out), "interpreter /lib") ||
		str.Contains(string(out), "interpreter /usr/lib64") {
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
	file = str.Replace(file, "*", ".*", -1)

	err := os.MkdirAll(fp.Dir(dest), 0750)
	errExit(err, "can't create dest dir: "+fp.Dir(dest))

	bb := "/home/xx/bin/busybox"
	c := bb + " grep \t" + file + " " + src + " > " + dest

	cmd := exec.Command(bb, "sh", "-c", c)
	out, err := cmd.CombinedOutput()

	errExit(err, "can't copy sha lines from "+src+" to "+dest+
		"\n"+string(out)+"\n"+str.Join(cmd.Args, " "))

	c = bb + " sed -i '\\|\t" + file + "|d' " + src

	cmd = exec.Command(bb, "sh", "-c", c)
	out, err = cmd.CombinedOutput()

	errExit(err, "can't remove sha lines from "+src+
		"\n"+string(out)+"\n"+str.Join(cmd.Args, " "))
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

func dumpSHA256(pkg pkgT) {
	files, err := walkDir(pkg.newPkgDir, "files")
	sort.Strings(files)
	remNewPkg(pkg, err)
	errExit(err, "can't get file list for: "+pkg.name)

	if len(files) == 0 {
		errExit(errors.New(""), "no files in pkg dir: "+pkg.newPkgDir)
	}

	var hashes string
	var sum string

	for _, file := range files {
		set, err := os.Stat(file)
		remNewPkg(pkg, err)
		errExit(err, "can't get file stat (broken link?): "+file)
		if set.IsDir() {
			continue
		}

		fd, err := os.Open(file)
		remNewPkg(pkg, err)
		errExit(err, "can't open file: "+file)

		hash := sha256.New()
		_, err = io.Copy(hash, fd)
		remNewPkg(pkg, err)
		errExit(err, "can't read file: "+file)
		fd.Close()

		sum = hex.EncodeToString(hash.Sum(nil))
		file = str.TrimPrefix(file, pkg.newPkgDir)
		hashes += fmt.Sprintf("%s\t%s\n", sum, file)
	}

	pathOut := fp.Join(pkg.progDir, "log", pkg.setVerNewRel, "sha256.log")
	fOut, err := os.Create(pathOut)
	errExit(err, "can't create hash log file")
	defer fOut.Close()

	fmt.Fprintf(fOut, "%s", hashes)
}

func getSharedLibs(file string) []string {
	var libs []string
	fd, err := os.Open(file)
	errExit(err, "can't open "+file)

	elfBin, err := elf.NewFile(fd)
	if err != nil {
		fd.Close()
		return libs
	}
	libs, err = elfBin.ImportedLibraries()
	errExit(err, "can't get imported libraries from "+file)
	fd.Close()

	return libs
}

// used only during build step
func (r *runT) dumpSharedLibs(pkg pkgT) {
	files, err := walkDir(pkg.pkgDir, "files")

	sharedLibs := make(map[string]bool)
	for _, file := range files {
		libs := getSharedLibs(file)
		for _, l := range libs {
			sharedLibs[l] = true
		}
	}

	if len(sharedLibs) == 0 {
		return
	}

	pathOut := fp.Join(pkg.progDir, "log", pkg.setVerRel, "shared_libs")
	fOut, err := os.Create(pathOut)
	errExit(err, "can't create shared libs file")
	defer fOut.Close()

	for lib := range sharedLibs {
		// exception for syslinux libraries
		if str.HasSuffix(lib, ".c32") {
			continue
		}

		libPath := r.findLibPath(lib)
		dep := r.world["/"].files[libPath]
		if libPath == "" {
			dep = pkg
		}
		fmt.Fprintf(fOut, "%s\t%s\t%s\t%s\t%s\n", lib, dep.name, dep.set, dep.ver, dep.rel)
	}
}

// used only during pkg build
func (r *runT) findLibPath(lib string) string {
	ldSoConf := fp.Join(r.rootDir, "/etc/ld.so.conf")
	if !fileExists(ldSoConf) {
		return ""
	}

	fd, err := os.Open(ldSoConf)
	errExit(err, "can't open ld.so.conf in "+ldSoConf)
	defer fd.Close()
	input := bufio.NewScanner(fd)

	for input.Scan() {
		ldLibraryPath := input.Text()
		libPath := fp.Join(ldLibraryPath, lib)
		_, found := r.world["/"].files[libPath]
		if found {
			return libPath
		}
	}

	return ""
}

func cleanup(pkg pkgT, pkgC pkgCfgT) {
	err := os.RemoveAll(pkgC.tmpDir)
	errExit(err, "can't remove tmp dir")

	pkgFiles, err := walkDir(pkg.newPkgDir, "files")
	errExit(err, "can't read pkg files")

	if !pkgC.crossBuild && !pkgC.muslBuild {
		rmStaticLibs(&pkgFiles)
	}
	stripDebug(&pkgFiles, pkg)

	rmEmptyLogs(pkg)
}

func moveLogs(pkg pkgT, pkgC pkgCfgT) {
	logDir := fp.Join(pkg.progDir, "log", pkg.setVerNewRel)
	err := os.RemoveAll(logDir)
	errExit(err, "can't remove existing log dir: "+logDir)

	cmd := exec.Command("/home/xx/bin/busybox", "cp", "-rd",
		pkgC.tmpLogDir, logDir)
	err = cmd.Run()
	errExit(err, "can't move log dir")
}

func (r *runT) saveHelp(pkg pkgT, pkgC pkgCfgT) {
	var c, helpType, file string
	switch {
	case fileExists(pkgC.steps.buildDir+"/configure") &&
		!str.Contains(pkgC.steps.configure, "meson"):

		helpType = "command"
		c = "./configure --help ||:"

	case fileExists(pkgC.steps.buildDir + "/meson.build"):
		helpType = "command"
		c = "meson configure ||:"

	case fileExists(pkgC.steps.buildDir + "/CMakeLists.txt"):
		helpType = "command"
		c = "cd build && cmake -LAH . | grep -v " + pkgC.tmpDir + " ||:"

	case fileExists(pkgC.steps.buildDir + "/wscript"):
		helpType = "command"
		c = "/usr/bin/waf configure --help"

	// mostly for dnsmasq
	case fileExists(pkgC.steps.buildDir + "/src/config.h"):
		helpType = "file"
		file = pkgC.steps.buildDir + "/src/config.h"

	// mostly for st and dwm
	case fileExists(pkgC.steps.buildDir + "/config.def.h"):
		helpType = "file"
		file = pkgC.steps.buildDir + "/config.def.h"

	// wpa_supplicant
	case fileExists(pkgC.steps.buildDir + "/wpa_supplicant/defconfig"):
		helpType = "file"
		file = pkgC.steps.buildDir + "/wpa_supplicant/defconfig"

	// hostapd
	case fileExists(pkgC.steps.buildDir + "/hostapd/defconfig"):
		helpType = "file"
		file = pkgC.steps.buildDir + "/hostapd/defconfig"

	default:
		return
	}

	pathOut := fp.Join(pkg.progDir, "log", pkg.setVerNewRel,
		"config-help.log")

	switch helpType {
	case "command":
		fOut, err := os.Create(pathOut)
		errExit(err, "can't create config help file")
		defer fOut.Close()

		cmd := r.prepareCmd(pkg, pkgC, "save_help", c,
			pkgC.steps.buildDir, fOut, fOut)
		err = cmd.Run()
		errExit(err, "can't execute config help")

	case "file":
		cmd := exec.Command("/home/xx/bin/busybox", "cp", file,
			pathOut)
		err := cmd.Run()
		errExit(err, "can't copy config-help file")
	}
}

func rmStaticLibs(pkgFiles *[]string) {
	for _, file := range *pkgFiles {
		if str.HasSuffix(file, ".la") {
			err := os.Remove(file)
			errExit(err, "can't remove "+file)
		}
	}
}

func rmEmptyLogs(pkg pkgT) {
	logFiles, err := walkDir(fp.Join(pkg.progDir, "log", pkg.setVerNewRel),
		"files")
	errExit(err, "can't read log files")
	for _, file := range logFiles {
		info, err := os.Stat(file)
		errExit(err, "can't read "+file)
		if info.Size() == 0 {
			err := os.Remove(file)
			errExit(err, "can't remove "+file)
		}
	}
}

func stripDebug(pkgFiles *[]string, pkg pkgT) {
	for _, file := range *pkgFiles {
		var lib, usrLib, bin bool
		ext := fp.Ext(file)

		// do not touch go packages, these are not static libraries
		if str.Contains(file, "/go/pkg") && str.HasSuffix(file, ".a") {
			continue
		}

		if str.HasPrefix(file, pkg.newPkgDir+"/lib/") {
			lib = true
		} else if str.HasPrefix(file, pkg.newPkgDir+"/usr/lib/") {
			usrLib = true
		}

		binDirs := []string{"/bin/", "/sbin/", "/usr/bin/",
			"/usr/sbin/", "/usr/libexec/", "/tools/bin"}
		for _, dir := range binDirs {
			if str.HasPrefix(file, pkg.newPkgDir+dir) {
				bin = true
				break
			}
		}

		if lib && ext == ".a" {
			runStrip("--strip-debug", file)
		} else if (usrLib || lib) && str.HasPrefix(ext, ".so") {
			runStrip("--strip-unneeded", file)
		} else if bin {
			// pie executables can't be stripped with --strip-all
			// relocation data is needed
			runStrip("--strip-unneeded", file)
		}
	}
}

func runStrip(arg, file string) {
	cmd := exec.Command("strip", arg, file)
	_, _ = cmd.Output()
}
