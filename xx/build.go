package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	fp "path/filepath"
	"sort"
	"strings"
)

func actionBuild(world map[string]worldT, genC genCfgT, pkgs []pkgT, pkgCfgs []pkgCfgT) {
	// todo: move this to a central createBuildDirs function
	err := os.MkdirAll("/tmp/xx/build", 0700)
	errExit(err, "can't create dir: /tmp/xx/build/")

	switch {
	case genC.buildEnv == "bootstrap" || genC.buildEnv == "bootstrap-base":
		fmt.Printf("\033[01m* processing %s...\033[00m\n",
			genC.setFileName)
		buildInstPkgs(world, genC, pkgs, pkgCfgs)
	default:
		buildSetFile(world, genC, pkgs, pkgCfgs)
	}
}

func buildSetFile(world map[string]worldT, genC genCfgT, pkgs []pkgT, pkgCfgs []pkgCfgT) {
	if len(pkgs) == 1 && strings.HasSuffix(pkgs[0].set, "_cross") {
		fmt.Printf("\033[01m* processing %s...\033[00m\n",
			genC.setFileName)
		buildInstPkgs(world, genC, pkgs, pkgCfgs)

		return
	}

	baseLinkFile := fp.Join(genC.rootDir, "base_linked")
	baseLinked := fileExists(baseLinkFile)
	baseBuild := genC.buildEnv == "base"
	alpineBuild := pkgs[0].categ == "alpine"

	// base env must exist first
	// todo: maybe move up and place in a function
	if !fileExists("/tmp/xx/base") && !baseBuild {
		fmt.Println("\033[01m* processing base.xx...\033[00m")

		baseGenC := genC
		baseGenC.buildEnv = "base"
		baseGenC.rootDir = "/tmp/xx/base"

		createRootDirs("/tmp/xx/base")
		baseFile := "/home/xx/set/build/base.xx"
		basePkgs, basePkgCfgs := parsePkgEnvFile(baseFile, baseGenC)
		for _, basePkgC := range basePkgCfgs {
			basePkgC.force = false
		}
		buildInstPkgs(world, baseGenC, basePkgs, basePkgCfgs)
		protectBaseDir()
	}

	if !baseLinked && !baseBuild && !alpineBuild {
		linkBaseDir(genC.rootDir)
	}

	fmt.Printf("\033[01m* processing %s...\033[00m\n", genC.setFileName)
	buildInstPkgs(world, genC, pkgs, pkgCfgs)

	if baseBuild {
		protectBaseDir()
	}
}

func linkBaseDir(rootDir string) {
	bb := "/home/xx/tools/busybox"
	cmd := exec.Command(bb, "cp", "-al", "/tmp/xx/base", rootDir)
	err := cmd.Run()
	errExit(err, "can't create link to /tmp/xx/base in:\n  "+rootDir)

	// remove the link to world dir in base system
	os.RemoveAll(rootDir + "/var/xx")

	baseLinkFile := fp.Join(rootDir, "base_linked")
	_, err = os.Create(baseLinkFile)
	errExit(err, "can't create base_linked file in "+baseLinkFile)
}

func protectBaseDir() {
	cmd := exec.Command("/home/xx/tools/busybox", "find", "/tmp/xx/base/",
		"-type", "f", "-exec", "chmod", "a-w", "{}", "+")
	err := cmd.Run()
	errExit(err, "can't remove write permissions in /tmp/xx/base")
}

func buildInstPkgs(world map[string]worldT, genC genCfgT, pkgs []pkgT, pkgCfgs []pkgCfgT) {
	for i, pkg := range pkgs {
		pkgC := pkgCfgs[i]
		freshlyBuilt := createPkg(world, genC, pkg, pkgC)

		if freshlyBuilt {
			// remove the old pkg from world
			delete(world["/"].pkgs, pkg)

			// clear verRel et al + get new values after the build
			pkg.setVerRel = ""
			progPkgDir := fp.Join(pkg.progDir, "pkg")
			pkg.rel, pkg.prevRel, pkg.newRel = getPkgRels(pkg)
			setVer := pkg.set + "-" + pkg.ver
			pkg.setVerRel = setVer + "-" + pkg.rel
			pkg.setVerPrevRel = setVer + "-" + pkg.prevRel
			pkg.setVerNewRel = setVer + "-" + pkg.newRel
			pkg.pkgDir = fp.Join(progPkgDir, pkg.setVerRel)
			pkg.newPkgDir = fp.Join(progPkgDir, pkg.setVerNewRel)

			// add a new pkg to root of the world; no cnt here as
			// only build step executes this
			addPkgToWorldT(world, pkg, "/")

			// double check if shared libraries are ok
			if genC.buildEnv != "bootstrap" {
				selfLibsExist(world, genC, pkg)
			}
		}

		if strings.HasSuffix(pkg.set, "_tools_cross") {
			continue
		} else if worldPkgExists(world, genC, pkg, pkgC) && !pkgC.force {
			continue
		} else if pkg.categ == "alpine" && !pkgC.cnt {
			continue
		} else {
			instPkg(pkg, pkgC, genC.rootDir)
			instPkgCfg(pkgC.cfgFiles, pkgC.instDir, genC.verbose)

			loc := "/"
			if pkgC.cnt {
				loc = pkgC.cntProg
			}
			addPkgToWorldT(world, pkg, loc)
		}
	}
}

// checks if the built pkg contains self-referencing shared libraries;
// these are assigned by defult so a check is necessary
func selfLibsExist(world map[string]worldT, genC genCfgT, pkg pkgT) {
	depLibs, _ := getPkgLibDeps(genC, pkg)
	files := world["/"].pkgFiles[pkg]
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

func instPkg(pkg pkgT, pkgC pkgCfgT, rootDir string) {
	// todo: can be removed once init of cnt dir is done elsewhere
	if pkgC.cnt {
		var dirsToCreate = map[string]fs.FileMode{
			fp.Join(pkgC.instDir, "/mnt"):                0777,
			fp.Join(rootDir, "/cnt/bin"):                 0755,
			fp.Join(rootDir, "/cnt/home/", pkgC.cntProg): 0755,
		}

		topDirs := []string{"/bin", "/etc", "/home", "/lib", "/lib64",
			"/sbin", "/usr", "/var/xx", "/mnt",
			"/dev", "/proc", "/run", "/sys", "/tmp"}

		for _, dir := range topDirs {
			dirsToCreate[fp.Join(pkgC.instDir, dir)] = 0755
		}

		for dir, perm := range dirsToCreate {
			err := os.MkdirAll(dir, perm)
			errExit(err, "can't create dir: "+dir)
		}

		fd, _ := os.Create(pkgC.instDir + "/config")
		fd.Close()

		ldLib := "ld-linux-x86-64.so.2"
		var symLinks = map[string]string{
			"/lib/" + ldLib:        "../usr/lib/" + ldLib,
			"/lib64/" + ldLib:      "../usr/lib/" + ldLib,
			"/usr/lib64":           "lib",
			"/cnt/bin/" + pkg.prog: "cntrun",
		}

		for symlink, target := range symLinks {
			l := fp.Join(pkgC.instDir, symlink)
			_ = os.Symlink(target, l)
		}

		// create symlinks to containers
		binCnt, _ := parseCntConf(rootDir + "/etc/cnt.conf")
		for bin, cnt := range binCnt {
			if cnt == pkg.prog {
				l := pkgC.instDir + "/../../bin/" + bin
				_ = os.Symlink("cntrun", l)
			}
		}

		cmd := exec.Command("/home/xx/tools/busybox", "cp",
			"/home/xx/xx/cntrun/cntrun", pkgC.instDir+"/../../bin/")
		if strings.Contains(pkgC.instDir, ":/") {
			cmd = exec.Command("scp", "-q",
				"/home/xx/xx/cntrun/cntrun",
				pkgC.instDir+"/../../bin/")
		}

		err := cmd.Run()
		errExit(err, "can't copy cntrun file")

		// install default shadow files
		cmd = exec.Command("/home/xx/tools/busybox", "cp",
			"/home/xx/prog/sys/shadow/cfg/std-latest/etc/perms",
			"/home/xx/prog/sys/shadow/cfg/std-latest/etc/group",
			"/home/xx/prog/sys/shadow/cfg/std-latest/etc/passwd",
			pkgC.instDir+"/etc/")
		if strings.Contains(pkgC.instDir, ":/") {
			etcDir := "/home/xx/prog/sys/shadow/cfg/std-latest/etc"
			cmd = exec.Command("scp", "-q",
				etcDir+"/perms",
				etcDir+"/group",
				etcDir+"/passwd",
				pkgC.instDir+"/etc/")
		}
		err = cmd.Run()
		errExit(err, "can't copy /etc/perms file to: "+pkgC.instDir+"/etc")
	}

	fmt.Println("  installing...")

	// todo: move this out of this function
	createRootDirs(rootDir)

	busybox := "/home/xx/tools/busybox"
	c := busybox + " cp -rf " + pkg.pkgDir + "/* " + pkgC.instDir
	if strings.Contains(pkgC.instDir, ":/") {
		c = "scp -q " + pkg.pkgDir + "/* " + pkgC.instDir
	}
	cmd := exec.Command(busybox, "sh", "-c", c)
	out, err := cmd.Output()
	errExit(err, "can't copy "+pkg.pkgDir+" to "+pkgC.instDir+
		"\n"+string(out)+"\n"+strings.Join(cmd.Args, " "))

	// todo: move this outside
	addPkgToWorldDir(pkg, pkgC.instDir)
}

// installs config files for the pkg
func instPkgCfg(cfgFiles map[string]string, instDir string, verbose bool) {
	if verbose && len(cfgFiles) > 0 {
		fmt.Println("  installing config files...")
	}

	var files []string
	for file := range cfgFiles {
		files = append(files, file)
	}
	sort.Strings(files)

	for _, file := range files {
		src := cfgFiles[file]
		dest := fp.Join(instDir, file)
		if verbose {
			fmt.Printf("    %s\n", file)
		}
		Cp(src, dest)
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

func addPkgToWorldT(world map[string]worldT, pkg pkgT, loc string) {
	// todo: read file from world dir not from /home/xx
	files, fileHash := getPkgFiles(pkg)
	for file, hash := range fileHash {
		world[loc].files[file] = pkg
		world[loc].fileHash[file] = hash
	}
	world[loc].pkgFiles[pkg] = files
	world[loc].pkgs[pkg] = true
}

func worldPkgExists(world map[string]worldT, genC genCfgT, pkg pkgT, pkgC pkgCfgT) bool {
	// todo: try to simplify this by making instDir == rootDir
	worldDir := fp.Join(genC.rootDir, "/var/xx")
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
	_, pkgInWorldT := world[loc].pkgs[pkg]

	if pkgInWorldDir && !pkgInWorldT {
		fmt.Println(pkg.name, pkg.setVerRel, "not consistent in the world")
		fmt.Println("in world dir:", pkgInWorldDir, f)
		fmt.Println("in world var, loc, len(deps):", pkgInWorldT, loc, len(world[loc].pkgs))
	}

	return pkgInWorldDir && pkgInWorldT
}

func createRootDirs(rootDir string) {
	var dirs = []string{
		"/{bin,etc,home,lib,lib64,mnt,root,sbin,usr,var}",
		"/{dev,proc,sys,run,tmp}",
		"/etc/{xx,rc.d,perms.d,sysctl.d}",
		"/home/{x,xx}",
		"/cnt/{bin,home,rootfs}",
		"/mnt/{misc,shared}",
		"/usr/lib/firmware",
		"/usr/{bin,include,lib,sbin,share}",
		"/usr/share/{doc,locale,man,misc,terminfo,zoneinfo}",
		"/usr/share/man/man{1,2,3,4,5,6,7,8}",
		"/run/lock",
		"/var/{cache,empty,lib,xx,log,spool,tmp}",
		"/var/lib/{misc,locate}",
	}
	for _, dir := range dirs {
		c := "mkdir -p " + rootDir + dir
		cmd := exec.Command("/home/xx/tools/ksh", "-c", c)
		out, err := cmd.CombinedOutput()
		errExit(err, "can't create dirs\n  "+string(out))
	}

	var modDirs = []string{
		"0750 /root",
		"0777 /mnt/shared",
		"1777 /tmp",
		"1777 /var/tmp",
	}
	for _, modDir := range modDirs {
		s := strings.Split(modDir, " ")
		mod := s[0]
		dir := s[1]
		cmd := exec.Command("/home/xx/tools/busybox", "chmod", mod,
			rootDir+dir)
		out, err := cmd.CombinedOutput()
		errExit(err, "can't change mode:\n  "+string(out))
	}

	var lnDirs = []string{
		"../run /var/",
		"../run/lock /var",
	}
	for _, lnDir := range lnDirs {
		d := strings.Split(lnDir, " ")
		cmd := exec.Command("/home/xx/tools/busybox", "ln", "-s", d[0],
			rootDir+d[1])
		_, _ = cmd.Output()
	}

	var symLinks = map[string]string{
		"/lib/ld-linux-x86-64.so.2":   "../usr/lib/ld-linux-x86-64.so.2",
		"/lib64/ld-linux-x86-64.so.2": "../usr/lib/ld-linux-x86-64.so.2",
		"/usr/lib64":                  "lib",
	}
	for symlink, target := range symLinks {
		_ = os.Symlink(target, rootDir+"/"+symlink)
	}

	if fileExists("/mnt/xx/boot") {
		err := os.MkdirAll(rootDir+"/mnt/xx/boot", 0700)
		errExit(err, "couldn't create "+rootDir+"/mnt/xx/boot")
	}

	_, _ = os.Create(fp.Join(rootDir, "/var/log/init.log"))
}
