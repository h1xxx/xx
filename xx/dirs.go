package main

import (
	"os"
	"os/exec"
	"path"

	fp "path/filepath"
	str "strings"
)

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
