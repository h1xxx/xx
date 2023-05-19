package main

import (
	"bufio"
	"crypto/sha256"
	"debug/elf"
	"encoding/hex"
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

	for i, p := range pkgs {
		pc := pkgCfgs[i]
		pc.instDir = r.baseDir

		// find the latest built pkg, don't build anything new
		if !fileExists(p.pkgDir) {
			p = getPkgPrevVer(p, r.re.gitVer)
			pc = r.getPkgCfg(p, "")
		}
		if !fileExists(p.pkgDir) {
			msg := "can't find previous pkg version for %s %s"
			errExit(fmt.Errorf(msg, p.name, p.ver))
		}

		fmt.Printf("+ %-32s %s\n", p.name, p.setVerRel)
		r.instPkg(p, pc, "/")

		// double check if shared libraries are ok
		if !r.isInit && !pc.muslBuild {
			r.selfLibsExist(p)
		}
	}

	baseOkFile := fp.Join(r.baseDir, "base_ok")
	_, err := os.Create(baseOkFile)
	errExit(err, "can't create base_ok file in "+baseOkFile)

	protectBaseDir(r.baseDir)
}

func (r *runT) buildPkgs() {
	for i, p := range r.pkgs {
		fmt.Printf("+ %-32s %s\n", p.name, p.setVerRel)
		pc := r.pkgCfgs[i]

		if pc.subPkg {
			// get the latest subpkg release in case when the main
			// pkg was rebuilt
			p.setVerRel = ""
			p.rel, p.prevRel, p.newRel = getPkgRels(p)
			p = getPkgSetVers(p)
			p = getPkgDirs(p)
		} else {
			p = r.buildPkg(p, pc)
		}

		if str.HasSuffix(p.set, "_tools_cross") {
			continue
		} else if r.worldPkgExists(p, pc) && !pc.force {
			continue
		} else if r.isSepSys && !pc.cnt {
			continue
		} else {
			r.instPkg(p, pc, "/")
			for _, s := range pc.steps.subPkgs {
				subPkg := getSubPkg(p, s.suffix)
				r.instPkg(subPkg, pc, "/")
			}

			// double check if shared libraries are ok
			if !r.isInit && !pc.muslBuild {
				r.selfLibsExist(p)
			}
		}
	}
}

func (r *runT) buildPkg(p pkgT, pc pkgCfgT) pkgT {
	if p.newRel != "00" && !pc.force || pc.src.srcType == "files" {
		return p
	}

	createPkgDirs(p, pc)
	r.getSrc(p, pc)

	r.execStep("prepare", p, pc)
	r.execStep("configure", p, pc)
	r.execStep("build", p, pc)
	r.execStep("pkg_create", p, pc)

	r.pkgBuildCheck(p, pc)
	moveLogs(p, pc)
	r.saveHelp(p, pc)
	cleanup(p, pc)
	dumpSHA256(p)

	if dirIsEmpty(p.newPkgDir) {
		fmt.Println("! pkg empty:", p.newPkgDir)
	}

	for _, s := range pc.steps.subPkgs {
		subPkg := getSubPkg(p, s.suffix)
		fmt.Printf("  creating subpkg %s...\n", subPkg.set)
		createSubPkg(p, subPkg, s.files)
	}

	// remove old pkg from world
	delete(r.world["/"].pkgs, p)
	for _, s := range pc.steps.subPkgs {
		subPkg := getSubPkg(p, s.suffix)
		delete(r.world["/"].pkgs, subPkg)
	}

	// get new release info after the build
	p.setVerRel = ""
	p.rel, p.prevRel, p.newRel = getPkgRels(p)
	p = getPkgSetVers(p)
	p = getPkgDirs(p)

	// dump shared libs for main pkg and for subpkgs
	if !pc.crossBuild {
		r.dumpSharedLibs(p)
	}
	for _, s := range pc.steps.subPkgs {
		subPkg := getSubPkg(p, s.suffix)
		r.dumpSharedLibs(subPkg)
		r.selfLibsExist(subPkg)
	}

	return p
}

func (r *runT) pkgBuildCheck(p pkgT, pc pkgCfgT) {
	var noNoDirs []string
	var noNoLibRe *regexp.Regexp

	muslNoNo := []string{"/usr", "/lib64", "/local", "/opt"}
	glibcNoNo := []string{"/bin", "/sbin", "/lib", "/lib64", "/opt",
		"/include", "/share", "/usr/lib64", "/usr/local"}

	switch {
	case pc.crossBuild && pc.muslBuild:
		return
	case pc.muslBuild:
		noNoDirs = muslNoNo
		noNoLibRe = r.re.noNoSharedLib
	default:
		noNoDirs = glibcNoNo
		noNoLibRe = r.re.noNoStaticLib
	}

	dirs, err := walkDir(p.newPkgDir, "dirs")
	errExit(err, "can't read pkg dirs in "+p.newPkgDir)

	for _, dir := range dirs {
		dir = str.TrimPrefix(dir, p.newPkgDir)
		msg := "WARNING! incorrect dir: %s\n"
		for _, noNo := range noNoDirs {
			if str.HasPrefix(dir, noNo) {
				fmt.Printf(msg, dir)
			}
		}
	}

	files, err := walkDir(p.newPkgDir, "files")
	errExit(err, "can't read pkg dirs in "+p.newPkgDir)
	for _, file := range files {
		f := str.TrimPrefix(file, p.newPkgDir)
		msg := "WARNING! incorrect lib: %s\n"

		isNoNoLib := noNoLibRe.MatchString(f)
		if isNoNoLib {
			fmt.Printf(msg, f)
		}

		isStaticBin := r.re.staticBin.MatchString(f)
		if isStaticBin && binHasInterpreter(file) {
			msg := "WARNING! non static bin: %s\n"
			fmt.Printf(msg, f)
		}

		isGlibcBin := r.re.glibcBin.MatchString(f)
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

func getSubPkg(p pkgT, suffix string) pkgT {
	subPkg := p
	subPkg.set = p.set + "_" + suffix
	subPkg = getPkgSetVers(subPkg)
	subPkg = getPkgDirs(subPkg)

	return subPkg
}

func createSubPkg(p, subPkg pkgT, files []string) {
	for _, f := range files {
		src := fp.Join(p.newPkgDir, f)
		dest := fp.Join(subPkg.newPkgDir, f)
		Mv(src, dest)

		RemEmptyDirs(fp.Dir(src))
		MoveShaInfo(p, subPkg, f)
	}
}

func MoveShaInfo(p, subPkg pkgT, file string) {
	src := fp.Join(p.progDir, "log", p.setVerNewRel, "sha256.log")
	dest := fp.Join(subPkg.progDir, "log", subPkg.setVerNewRel, "sha256.log")
	file = str.Replace(file, "*", ".*", -1)
	Mkdir(fp.Dir(dest))

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
func (r *runT) selfLibsExist(p pkgT) {
	depLibs, _ := r.getPkgLibDeps(p)
	files := r.world["/"].pkgFiles[p]
	for _, lib := range depLibs[p] {
		var found bool
		for _, file := range files {
			fileName := path.Base(file)
			if fileName == lib {
				found = true
				break
			}
		}
		if !found {
			errExit(ERR, "can't find shared lib for pkg: "+lib)
		}
	}
}

func dumpSHA256(p pkgT) {
	files, err := walkDir(p.newPkgDir, "files")
	sort.Strings(files)
	remNewPkg(p, err)
	errExit(err, "can't get file list for: "+p.name)

	if len(files) == 0 {
		errExit(ERR, "no files in pkg dir: "+p.newPkgDir)
	}

	var hashes string
	var sum string

	for _, file := range files {
		set, err := os.Stat(file)
		remNewPkg(p, err)
		errExit(err, "can't get file stat (broken link?): "+file)
		if set.IsDir() {
			continue
		}

		fd, err := os.Open(file)
		remNewPkg(p, err)
		errExit(err, "can't open file: "+file)

		hash := sha256.New()
		_, err = io.Copy(hash, fd)
		remNewPkg(p, err)
		errExit(err, "can't read file: "+file)
		fd.Close()

		sum = hex.EncodeToString(hash.Sum(nil))
		file = str.TrimPrefix(file, p.newPkgDir)
		hashes += fmt.Sprintf("%s\t%s\n", sum, file)
	}

	pathOut := fp.Join(p.progDir, "log", p.setVerNewRel, "sha256.log")
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

func (r *runT) dumpSharedLibs(p pkgT) {
	files, err := walkDir(p.pkgDir, "files")

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

	pathOut := fp.Join(p.progDir, "log", p.setVerRel, "shared_libs")
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
			dep = p
		}
		fmt.Fprintf(fOut, "%s\t%s\t%s\t%s\t%s\n",
			lib, dep.name, dep.set, dep.ver, dep.rel)
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

func cleanup(p pkgT, pc pkgCfgT) {
	err := os.RemoveAll(pc.tmpDir)
	errExit(err, "can't remove tmp dir")

	pkgFiles, err := walkDir(p.newPkgDir, "files")
	errExit(err, "can't read pkg files")

	if !pc.crossBuild && !pc.muslBuild {
		rmStaticLibs(&pkgFiles)
	}
	stripDebug(&pkgFiles, p)

	rmEmptyLogs(p)
}

func moveLogs(p pkgT, pc pkgCfgT) {
	logDir := fp.Join(p.progDir, "log", p.setVerNewRel)
	err := os.RemoveAll(logDir)
	errExit(err, "can't remove existing log dir: "+logDir)

	cmd := exec.Command("/home/xx/bin/busybox", "cp", "-rd",
		pc.tmpLogDir, logDir)
	err = cmd.Run()
	errExit(err, "can't move log dir")
}

func (r *runT) saveHelp(p pkgT, pc pkgCfgT) {
	var c, helpType, file string
	switch {
	case fileExists(pc.steps.buildDir+"/configure") &&
		!str.Contains(pc.steps.configure, "meson"):

		helpType = "command"
		c = "./configure --help ||:"

	case fileExists(pc.steps.buildDir + "/meson.build"):
		helpType = "command"
		c = "meson configure ||:"

	case fileExists(pc.steps.buildDir + "/CMakeLists.txt"):
		helpType = "command"
		c = "cd build && cmake -LAH . | grep -v " + pc.tmpDir + "||:"

	case fileExists(pc.steps.buildDir + "/wscript"):
		helpType = "command"
		c = "/usr/bin/waf configure --help"

	// mostly for dnsmasq
	case fileExists(pc.steps.buildDir + "/src/config.h"):
		helpType = "file"
		file = pc.steps.buildDir + "/src/config.h"

	// mostly for st and dwm
	case fileExists(pc.steps.buildDir + "/config.def.h"):
		helpType = "file"
		file = pc.steps.buildDir + "/config.def.h"

	// wpa_supplicant
	case fileExists(pc.steps.buildDir + "/wpa_supplicant/defconfig"):
		helpType = "file"
		file = pc.steps.buildDir + "/wpa_supplicant/defconfig"

	// hostapd
	case fileExists(pc.steps.buildDir + "/hostapd/defconfig"):
		helpType = "file"
		file = pc.steps.buildDir + "/hostapd/defconfig"

	default:
		return
	}

	pathOut := fp.Join(p.progDir, "log", p.setVerNewRel, "config-help.log")

	switch helpType {
	case "command":
		fOut, err := os.Create(pathOut)
		errExit(err, "can't create config help file")
		defer fOut.Close()

		cmd := r.prepareCmd(p, pc, "save_help", c,
			pc.steps.buildDir, fOut, fOut)
		err = cmd.Run()
		errExit(err, "can't execute config help")

	case "file":
		Cp(file, pathOut)
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

func rmEmptyLogs(p pkgT) {
	logFiles, err := walkDir(fp.Join(p.progDir, "log", p.setVerNewRel),
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

func stripDebug(pkgFiles *[]string, p pkgT) {
	for _, file := range *pkgFiles {
		var lib, usrLib, bin bool
		ext := fp.Ext(file)

		// do not touch go packages, these are not static libraries
		if str.Contains(file, "/go/pkg") && str.HasSuffix(file, ".a") {
			continue
		}

		if str.HasPrefix(file, p.newPkgDir+"/lib/") {
			lib = true
		} else if str.HasPrefix(file, p.newPkgDir+"/usr/lib/") {
			usrLib = true
		}

		binDirs := []string{"/bin/", "/sbin/", "/usr/bin/",
			"/usr/sbin/", "/usr/libexec/", "/tools/bin"}
		for _, dir := range binDirs {
			if str.HasPrefix(file, p.newPkgDir+dir) {
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
