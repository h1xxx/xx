package main

import (
	"io/fs"
	"os"
	"os/exec"

	fp "path/filepath"
	str "strings"
)

func (r *runT) createRootDirs() {
	var dirs = []string{
		"/root",
		"/{dev,proc,sys,run,tmp}",
		"/home/{x,xx}",
		"/mnt/{misc,shared}",
		"/run/{lock,pid}",
		"/var/{cache,empty,xx,log,spool,tmp}",
		"/var/lib/{misc,locate}",
	}

	for _, dir := range dirs {
		Mkdir(fp.Join(r.rootDir, dir))
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
			r.rootDir+dir)
		out, err := cmd.CombinedOutput()
		errExit(err, "can't change mode:\n  "+string(out))
	}

	var lnDirs = map[string]string{
		"../run":      fp.Join(r.rootDir, "/var/run"),
		"../run/lock": fp.Join(r.rootDir, "/var/lock"),
	}

	for target, dest := range lnDirs {
		Symlink(target, dest)
	}

	if r.installCnt {
		Mkdir(fp.Join(r.rootDir, "/cnt/{bin,home,rootfs}"))
		Cp("/home/xx/bin/crun", fp.Join(r.rootDir, "/bin"))
		for _, cntProg := range r.cnts {
			r.createCntDirs(cntProg)
		}
	}

	if fileExists("/mnt/xx/boot") {
		err := os.MkdirAll(r.rootDir+"/mnt/xx/boot", 0700)
		errExit(err, "couldn't create "+r.rootDir+"/mnt/xx/boot")
	}

	fd, _ := os.Create(fp.Join(r.rootDir, "/var/log/init.log"))
	fd.Close()
}

func (r *runT) createCntDirs(cntProg string) {
	dirs := make(map[string]fs.FileMode)

	instDir := fp.Join(r.rootDir, "/cnt/rootfs/", cntProg)

	dirs[fp.Join(instDir, "/mnt/shared")] = 0770
	dirs[fp.Join(r.rootDir, "/cnt/home/", cntProg, "work_dir")] = 0750

	topDirs := []string{"/var/xx", "/bind", "/home/cnt",
		"/dev", "/proc", "/run", "/sys", "/tmp"}

	for _, dir := range topDirs {
		dirs[fp.Join(instDir, dir)] = 0750
	}

	for dir, perm := range dirs {
		err := os.MkdirAll(dir, perm)
		errExit(err, "can't create dir: "+dir)
	}

	fd, _ := os.Create(instDir + "/config")
	fd.Close()

	// install default system files
	Cp("/home/xx/cfg/sys/*", instDir+"/")

	fd, _ = os.Create(fp.Join(r.rootDir, "/cnt/home", cntProg, ".profile"))
	fd.WriteString("cd /home/cnt/work_dir\n")
	fd.Close()
}

func (r *runT) createBuildDirs() {
	err := os.MkdirAll("/tmp/xx/build", 0700)
	errExit(err, "can't create dir: /tmp/xx/build/")

	err = os.MkdirAll(r.rootDir, 0700)
	errExit(err, "can't create dir: "+r.rootDir)
}

func createPkgDirs(pkg pkgT, pkgC pkgCfgT) {
	dirs := []string{pkgC.tmpDir, pkgC.tmpLogDir}
	for _, d := range dirs {
		err := os.MkdirAll(d, 0700)
		errExit(err, "can't create tmp dir: "+d)
	}

	dirs = []string{"pkg", "src", "log"}
	for _, d := range dirs {
		dir := fp.Join(pkg.progDir, d)
		err := os.MkdirAll(dir, 0700)
		errExit(err, "can't create pkg dir: "+dir)
	}
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
		target := str.TrimPrefix(file, pkgDir+"/usr")
		dest := fp.Join(instDir, target)

		target = "../usr" + target
		Symlink(target, dest)
	}

	// todo: also handle remote hosts
}
