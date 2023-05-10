package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"sort"
	"time"
	"unicode"

	fp "path/filepath"
	str "strings"
)

// type can be: "all", "files", "sysfiles", "dirs"
func walkDir(rootDir, fType string) ([]string, error) {
	var names []string
	var getAll, getFiles, sysFiles, getDirs bool

	switch fType {
	case "all":
		getAll = true
	case "files":
		getFiles = true
	case "sysfiles":
		getFiles = true
		sysFiles = true
	case "dirs":
		getDirs = true
	}

	if !fileExists(rootDir) {
		errExit(errors.New(""), "dir doesn't exist:\n  "+rootDir)
	}

	err := fp.Walk(rootDir,
		func(path string, linfo os.FileInfo, err error) error {
			switch {
			case str.HasPrefix(path, "/proc/"):
				return nil

			case str.HasPrefix(path, "/sys/"):
				return nil

			case str.HasPrefix(path, "/dev/"):
				return nil

			case str.HasPrefix(path, "/run/"):
				return nil

			case sysFiles && str.HasPrefix(path, "/root/"):
				return nil

			case sysFiles && str.HasPrefix(path, "/mnt/"):
				return nil

			case sysFiles && str.HasPrefix(path, "/home/"):
				return nil
			}

			info, errInfo := os.Stat(path)
			if errors.Is(errInfo, fs.ErrNotExist) {
				fmt.Fprintf(os.Stderr, "bad file: %s (%+v)\n",
					path, errors.Unwrap(errInfo))
				return nil
			}

			if errInfo != nil {
				return errInfo
			}

			switch {
			case !info.IsDir() && info.Mode().IsRegular() &&
				(getAll || getFiles):

				names = append(names, path)

			case info.IsDir() && (getAll || getDirs):
				names = append(names, path)
			}

			if errors.Is(err, fs.ErrNotExist) {
				fmt.Fprintf(os.Stderr, "bad file: %s (%+v)\n",
					path, errors.Unwrap(err))
			}

			if err != nil {
				return err
			}

			return nil
		})
	return names, err
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

func stringExists(s string, slice []string) bool {
	idx := sort.SearchStrings(slice, s)
	if idx < len(slice) && slice[idx] == s {
		return true
	}
	return false
}

func fileExists(fPath string) bool {
	if str.Contains(fPath, ":/") {
		split := str.Split(fPath, ":")
		host := split[0]
		path := split[1]
		c := "ssh " + host + " stat " + path
		cmd := exec.Command("/home/xx/bin/busybox", "sh", "-c", c)
		err := cmd.Run()
		if err != nil {
			return false
		}
	} else {
		_, err := os.Stat(fPath)
		if errors.Is(err, os.ErrNotExist) {
			return false
		}
	}
	return true
}

func dirIsEmpty(dir string) bool {
	fd, err := os.Open(dir)
	errExit(err, "can't open dir: "+dir)
	defer fd.Close()

	_, err = fd.Readdirnames(1)
	if err == io.EOF {
		return true
	}
	errExit(err, "can't read dir content: "+dir)

	return false
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func Cp(src, dest string) {
	dest = fp.Dir(dest)
	err := os.MkdirAll(dest, 0750)
	errExit(err, "can't create dest dir: "+dest)

	bb := "/home/xx/bin/busybox"
	c := bb + " cp -rf " + src + " " + dest
	if str.Contains(dest, ":/") {
		c = "scp -q " + src + " " + dest
	}

	cmd := exec.Command("/home/xx/bin/bash", "-c", c)
	out, err := cmd.CombinedOutput()

	errExit(err, "can't copy "+src+" to "+dest+
		"\n"+string(out)+"\n"+str.Join(cmd.Args, " "))
}

func Mv(src, dest string) {
	dest = fp.Dir(dest)
	err := os.MkdirAll(dest, 0750)
	errExit(err, "can't create dest dir: "+dest)

	bb := "/home/xx/bin/busybox"
	c := bb + " mv -f " + src + " " + dest

	cmd := exec.Command("/home/xx/bin/bash", "-c", c)
	out, err := cmd.CombinedOutput()

	errExit(err, "can't move "+src+" to "+dest+
		"\n"+string(out)+"\n"+str.Join(cmd.Args, " "))
}

func Symlink(src, dest string) {
	destDir := fp.Dir(dest)
	err := os.MkdirAll(destDir, 0750)
	errExit(err, "can't create dest dir: "+destDir)

	bb := "/home/xx/bin/busybox"
	c := bb + " ln -sf " + src + " " + dest

	cmd := exec.Command("/home/xx/bin/bash", "-c", c)
	out, err := cmd.CombinedOutput()

	errExit(err, "can't symlink "+dest+" to "+src+
		"\n"+string(out)+"\n"+str.Join(cmd.Args, " "))
}

func Mkdir(dir string) {
	bb := "/home/xx/bin/busybox"
	c := bb + " mkdir -p " + dir

	cmd := exec.Command("/home/xx/bin/bash", "-c", c)
	out, err := cmd.CombinedOutput()

	errExit(err, "can't create dir\n  "+string(out))
}

func RemEmptyDirs(dir string) {
	bb := "/home/xx/bin/busybox"
	c := bb + " rmdir -p --ignore-fail-on-non-empty " + dir

	cmd := exec.Command(bb, "sh", "-c", c)
	out, err := cmd.CombinedOutput()

	errExit(err, "can't remove empty dir "+dir+"\n"+string(out)+"\n")
}

func isSymLink(file string) bool {
	fStat, err := os.Stat(file)
	if err != nil {
		return true
	}

	return fStat.Mode()&os.ModeSymlink != 0
}

func uniqueSlice(s []string) []string {
	var res []string
	l := len(s)
	for i, v := range s {
		if v != s[i+1] {
			res = append(res, v)
		}
		if i == l-2 && v != s[i+1] {
			res = append(res, s[i+1])
		}
		if i == l-2 {
			break
		}
	}
	return res
}

func strDigitsOnly(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func getCurrentDate() string {
	t := time.Now()
	return fmt.Sprintf("%d-%.2d-%.2d", t.Year(), t.Month(), t.Day())
}

func errExit(err error, msg string) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "\n  error: "+msg)
		fmt.Fprintf(os.Stderr, "  %s\n", err)
		os.Exit(1)
	}
}
