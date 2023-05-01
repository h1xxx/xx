package main

import (
	"bufio"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path"
	"regexp"
	"syscall"

	fp "path/filepath"
	str "strings"
)

// todo:
// add +no_write_conf
// add +shell
// add -links create in /cnt/bin
// check if fileAbs is readable by container
// move/link newly created files to current dir
// add custom /etc/conf loc

//case prog == "startx":
//	cmdLine += "-- vt4"

type runT struct {
	cnt  string
	bin  string
	args []string

	cntCfgFile string

	cntId      int
	cntConfStr string
	cntConf    cntConfT
	cntNetwork string
	cntIP      string

	dirs        dirsT
	bindTargets []string

	lxcConfig string

	debug    bool
	download bool
	link     bool
}

type cntConfT struct {
	tty0   bool
	tty4   bool
	fb     bool
	dri    bool
	snd    bool
	input  bool
	net    bool
	udev   bool
	sdl    bool
	shared bool
}

type dirsT struct {
	cnt  string
	home string
	bind string
}

func main() {
	var r runT
	r.cntCfgFile = "/etc/cnt.conf"
	r.parseArgs()
	r.parseConf()

	syscall.Umask(0)

	r.dirs.cnt = "/cnt/rootfs/" + r.cnt
	r.dirs.home = fp.Join("/cnt/home/", r.cnt)
	r.dirs.bind = fp.Join(r.dirs.cnt, "bind")

	if r.debug {
		r.printDebug()
	}

	if r.bin == "crun" {
		switch {
		case r.link:
			r.createLinks()
		}
		return
	}

	r.doChecks()

	// remove all files in a dir for binds
	clearDir(r.dirs.bind, r.debug)

	// create lxc configuration
	r.makeLxcConfig()

	// write lxc configuration
	r.writeLxcConfig()

	// write cmd file
	r.writeCmd()

	// run
	env := []string{
		"TERM=linux",
		"HOME=/home/x",
		"PATH=/bin:/sbin:/usr/bin:/usr/sbin",
		"LC_ALL=en_US.utf8",
	}

	argv := []string{"-n", r.cnt, "-P", "/cnt/rootfs"}

	err := syscall.Exec("/bin/lxc-execute", argv, env)
	errExit(err)
}

func (r *runT) doChecks() {
	userIdWant := "1000"

	userCur, err := user.Current()
	errExit(err)

	userWant, _ := user.LookupId(userIdWant)

	if r.bin != "crun" && userCur.Uid != userIdWant {
		errExit(fmt.Errorf("run program as user \"%s\"",
			userWant.Username))
	}

	if r.bin != "crun" && r.cnt == "" {
		r.printDebug()
		errExit(fmt.Errorf("no container detected"))
	}
}

func (r *runT) createLinks() {
	fd, err := os.Open(r.cntCfgFile)
	errExit(err)
	defer fd.Close()

	clearLinks()

	var binSection bool
	reWSpace := regexp.MustCompile(`\s+`)

	input := bufio.NewScanner(fd)
	for input.Scan() {
		line := str.TrimSpace(input.Text())

		if line == "" || line[0] == '#' {
			continue
		}

		if line == "[ container bins ]" {
			binSection = true
			continue
		}

		if !binSection {
			continue
		}

		if str.HasPrefix(line, "[") {
			break
		}

		_, bin := getKeyVal(line, reWSpace)
		target := fp.Join("/cnt/bin", bin)

		if bin == "crun" {
			continue
		}

		pr("creating symlink %s => crun", target)
		err := os.Symlink("crun", target)
		errExit(err)
	}
}

func clearLinks() {
	files, err := os.ReadDir("/cnt/bin")
	errExit(err)
	pr("removing symlinks...")

	for _, file := range files {
		if file.Name() != "crun" {
			err := os.Remove(fp.Join("/cnt/bin", file.Name()))
			errExit(err)
		}
	}
}

func fileExists(arg string) bool {
	_, err := os.Stat(arg)
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	return true
}

func isDir(p string) bool {
	pStat, err := os.Stat(p)
	errExit(err)
	if pStat.IsDir() {
		return true
	}
	return false
}

func clearDir(dir string, debug bool) {
	if debug {
		pr("debug: clearing dir %s...", dir)
	}

	names, err := ioutil.ReadDir(dir)
	errExit(err)

	for _, entry := range names {
		err := os.RemoveAll(path.Join([]string{dir, entry.Name()}...))
		errExit(err)
	}
}

func errExit(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`
usage: cntrun <program> [program args]
-n	disable net at all times (not implemented yet)

link name to cntrun is also interpreted as a program name
`)
}
