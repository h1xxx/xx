package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"sort"

	fp "path/filepath"
	str "strings"
)

func (r *runT) actionBuild() {
	// todo: move this to a central createBuildDirs function
	err := os.MkdirAll("/tmp/xx/build", 0700)
	errExit(err, "can't create dir: /tmp/xx/build/")

	switch {
	case r.isInit:
		prHigh("processing %s...", r.buildEnv)
		r.buildInstPkgs()
	default:
		r.buildSetFile()
	}
}

func (r *runT) buildSetFile() {
	if len(r.pkgs) == 1 && str.HasSuffix(r.pkgs[0].set, "_cross") {
		prHigh("processing %s...", r.buildEnv)
		r.buildInstPkgs()

		return
	}

	err := os.MkdirAll(r.rootDir, 0700)
	errExit(err, "can't create dir: "+r.rootDir)

	baseLinkFile := fp.Join(r.rootDir, "base_linked")
	baseLinked := fileExists(baseLinkFile)
	baseOkFile := fp.Join(r.baseDir, "base_ok")
	baseOk := fileExists(baseOkFile)

	// todo: move this to runT, maybe name as r.isSepEnv
	alpineBuild := r.pkgs[0].categ == "alpine"

	// base env must exist first, copy it if it's not in place
	if !baseOk && !r.isInit {
		prHigh("copying base.xx...")
		r.installBase()
	}

	if !baseLinked && !r.baseEnv && !alpineBuild {
		linkBaseDir(r.rootDir, r.baseDir)
	}

	prHigh("processing %s...", r.buildEnv)
	r.buildInstPkgs()

	if r.baseEnv {
		_, _ = os.Create(baseOkFile)
		protectBaseDir(r.baseDir)
	}
}

func (r *runT) installBase() {
	baseRunCfg := *r
	baseRunCfg.rootDir = r.baseDir

	createRootDirs(r.baseDir)
	pkgs, pkgCfgs := baseRunCfg.parseBuildEnvFile(r.baseFile)

	// find the latest built pkg, don't build anything new
	for i, pkg := range pkgs {
		pkgC := pkgCfgs[i]

		if !fileExists(pkg.pkgDir) {
			pkg = getPkgPrevVer(pkg)
			pkgC = r.getPkgCfg(pkg, "")
		}

		if !fileExists(pkg.pkgDir) {
			msg := "can't find previous pkg version for %s %s"
			errExit(fmt.Errorf(msg, pkg.name, pkg.ver), "")
		}

		fmt.Printf("+ %-32s %s\n", pkg.name, pkg.setVerRel)
		instPkg(pkg, pkgC, r.baseDir)
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

func linkBaseDir(rootDir, baseDir string) {
	bb := "/home/xx/bin/busybox"
	c := bb + " cp -al " + baseDir + "/* " + rootDir + "/"
	cmd := exec.Command(bb, "sh", "-c", c)
	err := cmd.Run()
	errExit(err, "can't create link to "+baseDir+" in:\n  "+rootDir)

	// remove the link to world dir in base system
	os.RemoveAll(rootDir + "/var/xx")

	baseLinkFile := fp.Join(rootDir, "base_linked")
	_, err = os.Create(baseLinkFile)
	errExit(err, "can't create base_linked file in "+baseLinkFile)
}

func protectBaseDir(baseDir string) {
	cmd := exec.Command("/home/xx/bin/busybox", "find", baseDir,
		"-type", "f", "-exec", "chmod", "a-w", "{}", "+")
	err := cmd.Run()
	errExit(err, "can't remove write permissions in "+baseDir)
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
			instPkg(pkg, pkgC, r.rootDir)
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

func instPkg(pkg pkgT, pkgC pkgCfgT, rootDir string) {
	// todo: can be removed once init of cnt dir is done elsewhere
	if pkgC.cnt {
		var dirsToCreate = map[string]fs.FileMode{
			fp.Join(pkgC.instDir, "/mnt"):                0777,
			fp.Join(rootDir, "/cnt/bin"):                 0755,
			fp.Join(rootDir, "/cnt/home/", pkgC.cntProg): 0755,
		}

		topDirs := []string{"/home/cnt", "/var/xx", "/mnt/shared",
			"/dev", "/proc", "/run", "/sys", "/tmp",
			"/files"}

		for _, dir := range topDirs {
			dirsToCreate[fp.Join(pkgC.instDir, dir)] = 0755
		}

		for dir, perm := range dirsToCreate {
			err := os.MkdirAll(dir, perm)
			errExit(err, "can't create dir: "+dir)
		}

		fd, _ := os.Create(pkgC.instDir + "/config")
		fd.Close()

		var symLinks = map[string]string{
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

		cmd := exec.Command("/home/xx/bin/busybox", "cp",
			"/home/xx/xx/cntrun/cntrun", pkgC.instDir+"/../../bin/")
		if str.Contains(pkgC.instDir, ":/") {
			cmd = exec.Command("scp", "-q",
				"/home/xx/xx/cntrun/cntrun",
				pkgC.instDir+"/../../bin/")
		}

		err := cmd.Run()
		errExit(err, "can't copy cntrun file")

		// install default system files
		Cp("/home/xx/cfg/sys/*", pkgC.instDir+"/")
	}

	fmt.Printf("  installing...\n")

	// todo: move this out of this function
	createRootDirs(rootDir)

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

func createBinLinks(pkgDir, instDir string) {
	var files []string

	srcDir := fp.Join(pkgDir, "/usr/bin")
	if fileExists(srcDir) {
		binFiles, err := walkDir(srcDir, "files")
		errExit(err, "can't get file list for bin dir")
		files = append(files, binFiles...)
	}

	srcDir = fp.Join(pkgDir, "/usr/sbin")
	if fileExists(srcDir) {
		sbinFiles, err := walkDir(srcDir, "files")
		errExit(err, "can't get file list for sbin dir")
		files = append(files, sbinFiles...)
	}

	for _, file := range files {
		f := str.TrimPrefix(file, pkgDir+"/usr")
		dest := fp.Join(instDir, f)

		destDir := path.Dir(dest)
		err := os.MkdirAll(destDir, 0755)
		errExit(err, "can't create dest dir "+destDir)

		busybox := "/home/xx/bin/busybox"
		c := busybox + " ln -fs ../usr" + f + " " + dest

		cmd := exec.Command(busybox, "sh", "-c", c)
		out, err := cmd.CombinedOutput()
		errExit(err, "can't create links in "+dest+"\n"+string(out))
	}

	// todo: also handle remote hosts
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

func createRootDirs(rootDir string) {
	var dirs = []string{
		"/root",
		"/{dev,proc,sys,run,tmp}",
		"/home/{x,xx}",
		"/cnt/{bin,home,rootfs/common}",
		"/mnt/{misc,shared}",
		"/run/{lock,pid}",
		"/var/{cache,empty,xx,log,spool,tmp}",
		"/var/lib/{misc,locate}",
	}

	for _, dir := range dirs {
		c := "mkdir -p " + rootDir + dir
		cmd := exec.Command("/home/xx/bin/bash", "-c", c)
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
		s := str.Split(modDir, " ")
		mod := s[0]
		dir := s[1]
		cmd := exec.Command("/home/xx/bin/busybox", "chmod", mod,
			rootDir+dir)
		out, err := cmd.CombinedOutput()
		errExit(err, "can't change mode:\n  "+string(out))
	}

	var lnDirs = []string{
		"../run /var/",
		"../run/lock /var",
	}

	for _, lnDir := range lnDirs {
		d := str.Split(lnDir, " ")
		cmd := exec.Command("/home/xx/bin/busybox", "ln", "-s", d[0],
			rootDir+d[1])
		_, _ = cmd.Output()
	}

	if fileExists("/mnt/xx/boot") {
		err := os.MkdirAll(rootDir+"/mnt/xx/boot", 0700)
		errExit(err, "couldn't create "+rootDir+"/mnt/xx/boot")
	}

	_, _ = os.Create(fp.Join(rootDir, "/var/log/init.log"))
}
