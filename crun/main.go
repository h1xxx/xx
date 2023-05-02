package main

import (
	"bufio"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"os/user"
	"path"
	"regexp"
	"syscall"

	fp "path/filepath"
	str "strings"
)

// todo:
// move/link newly created files to current dir
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
	bindWork    []string

	lxcConfig string

	debug    bool
	link     bool
	shell    bool
	writeCfg bool
	get      bool
	getDest  string
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
	r.writeCfg = true
	r.parseArgs()
	if !r.shell {
		r.writeCfg = true
	}
	r.parseConf()

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

	if r.get {
		r.getFiles()
		return
	}

	r.doChecks()

	// remove all files in a dir for bind mounts
	clearDir(r.dirs.bind, r.debug)

	// create lxc configuration
	r.makeLxcConfig()

	// write lxc configuration
	if r.writeCfg {
		r.writeLxcConfig()
	}

	// write cmd file
	r.writeCmd()

	// create bind mount points
	oldUmask := syscall.Umask(0)
	r.createBindDirs()
	syscall.Umask(oldUmask)

	// run
	env := []string{
		"TERM=linux",
		"HOME=/home/x",
		"PATH=/bin:/sbin:/usr/bin:/usr/sbin",
		"LC_ALL=en_US.utf8",
	}

	argv := []string{"-n", r.cnt, "-P", "/cnt/rootfs"}

	if r.shell {
		argv = append(argv, []string{"--", "/bin/bash", "-l"}...)
	}

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

func (r *runT) createBindDirs() {
	for _, path := range r.bindTargets {
		bindFullDir := fp.Join(r.dirs.bind, fp.Dir(path))

		// todo: use 770, add cnt user to x group, change umask first
		err := os.MkdirAll(bindFullDir, 0777)
		errExit(err)
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

func (r *runT) getFiles() {
	if r.getDest == "" || r.getDest[0] == '+' || r.getDest[0] == '-' {
		errExit(fmt.Errorf("target dir not specified"))
	}

	r.clearWorkDir()

	err := os.MkdirAll(r.getDest, 0777)
	errExit(err)

	workDir := fp.Join(r.dirs.home, "work_dir")
	files, err := os.ReadDir(workDir)
	errExit(err)

	for _, file := range files {
		path := fp.Join(workDir, file.Name())

		cmd := exec.Command("cp", "-vrd",
			"--preserve=all", "--no-preserve=ownership",
			path, r.getDest)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		err := cmd.Run()
		errExit(err)
	}

	clearDir(workDir, r.debug)
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
		prD("clearing dir %s...", dir)
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
