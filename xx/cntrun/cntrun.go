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
	"path/filepath"
	"regexp"
	"strings"
)

func main() {
	user, err := user.Current()
	errExit(err, "can't get user ID")

	if user.Uid != "1000" {
		errExit(errors.New(""), "run this program as uid 1000")
	}

	binCnt, cntIP := parseCntConf("/etc/cnt.conf")
	var prog string
	var initArgs []string

	// get program name also from links to cntrun binary
	if filepath.Base(os.Args[0]) == "cntrun" {
		prog = os.Args[1]
		if len(os.Args) > 2 {
			initArgs = os.Args[2:]
		}
	} else {
		prog = filepath.Base(os.Args[0])
		if len(os.Args) > 1 {
			initArgs = os.Args[1:]
		}
	}

	homeDir := "/usr/cnt/" + binCnt[prog] + "/home"
	filesDir := "/usr/cnt/" + binCnt[prog] + "/files"
	paths := getPathsFromArgs(initArgs)

	// remove all files in a dir for links
	clearDir(homeDir)

	// create lxc configuration
	lxcConfig := makeLxcConfig(binCnt[prog], cntIP[binCnt[prog]],
		netEnable(initArgs))

	// create links or bind configs for all files and dirs defined in
	// program arguments
	for _, p := range paths {
		fileBase := filepath.Base(p)
		fileBase = strings.Replace(fileBase, "'", "", -1)
		cntLink := path.Join(filesDir, fileBase)
		// try hard links first; this has the advantage of bypassing
		// parent dir permissions
		err := createHardLink(p, cntLink)

		// if hard links are not possible due to different filesystems
		// or other reasons, then try to bind files/dirs 
		fmt.Println(err, p, cntLink)
		if err != nil {
			bindFile(p, cntLink, &lxcConfig)
		}
	}

	// bind /var/lib/<prog> to /home in a container
	progVarDir := "/var/lib/" + binCnt[prog]
	if fileExists(progVarDir) {
		bindFile(progVarDir, "/home", &lxcConfig)
	}

	// prepare cmd
	var args []string
	for _, arg := range initArgs {
		if fileExists(arg) {
			fileBase := filepath.Base(arg)
			fileBase = strings.Replace(fileBase, "'", "", -1)
			link := "'/files/" + fileBase + "'"
			args = append(args, link)
		} else {
			if strings.Contains(arg, "'") {
				errExit(errors.New(""),
					"arg contains invalid char: '")
			}
			args = append(args, arg)
		}
	}

	cmdLine := "/bin/" + prog + " " + strings.Join(args, " ")

	// program specific arguments
	switch {

	// changes dir to the one that's shared between host and container,
	// so  the downloaded files can be visible by host
	case prog == "w3m" || prog == "youtube-dl" || prog == "mutt":
		cmdLine = "/bin/ksh -c \"cd /mnt/shared && " + cmdLine + "\""
		lxcConfig += cfgShared

	case prog == "startx":
		cmdLine += "-- vt4"
	}

	cfgCmd := "\nlxc.execute.cmd = " + cmdLine
	lxcConfig += cfgCmd

	// write config
	fOpts := os.O_CREATE | os.O_TRUNC | os.O_WRONLY
	fd, err := os.OpenFile("/usr/cnt/"+binCnt[prog]+"/config", fOpts, 0600)
	errExit(err, "can't write lxc config to /usr/cnt/"+prog+"/config")
	fmt.Fprintf(fd, lxcConfig)

	// run
	cmd := exec.Command("lxc-execute", "-n", binCnt[prog], "-P", "/usr/cnt")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	errExit(err, "can't run")

	// cleanup
	clearDir(filesDir)
}

func parseCntConf(cntConf string) (map[string]string, map[string]string) {
	binCnt := make(map[string]string)
	cntIP := make(map[string]string)
	reWSpace := regexp.MustCompile(`\s+`)

	f, err := os.Open(cntConf)
	errExit(err, "can't open file: "+cntConf)
	defer f.Close()

	input := bufio.NewScanner(f)
	for input.Scan() {
		line := input.Text()

		if line == "" || string(line[0]) == "#" {
			continue
		}

		line = reWSpace.ReplaceAllString(line, "\t")
		l := strings.Split(line, "\t")

		switch {
		case len(l) < 3:
			errExit(errors.New(""),
				"too few fields in line:\n  "+line)
		case len(l) > 4:
			errExit(errors.New(""),
				"too many fields in line:\n  "+line)
		}
		binCnt[l[0]] = l[1]
		cntIP[l[1]] = l[2]
	}

	return binCnt, cntIP
}

func netEnable(initArgs []string) bool {
	for _, arg := range initArgs {
		switch {
		case strings.Contains(arg, "http"):
			return true
		case strings.Contains(arg, "youtube"):
			return true
		case strings.Contains(arg, "invidious"):
			return true
		}
	}

	return false
}

func getPathsFromArgs(initArgs []string) []string {
	var paths []string

	for _, arg := range initArgs {
		if fileExists(arg) {
			paths = append(paths, arg)
		}
	}

	return paths
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
	errExit(err, "can't get stat:\n  "+p)
	if pStat.IsDir() {
		return true
	}
	return false
}

func clearDir(dir string) {
	names, err := ioutil.ReadDir(dir)
	errExit(err, "can't read files from "+dir)

	for _, entry := range names {
		os.RemoveAll(path.Join([]string{dir, entry.Name()}...))
	}
}

func bindFile(p, cntLink string, lxcConfig *string) {
	fileBase := filepath.Base(p)
	fileBase = strings.Replace(fileBase, "'", "", -1)
	targetDir := "files/"
	if cntLink == "/home" {
		fileBase = ""
		targetDir = "home/"
	}

	fileAbs, err := filepath.Abs(p)
	errExit(err, "can't get absolute path for "+p)

	// todo: check if fileAbs is readable by container

	pType := "file"
	if isDir(p) {
		pType = "dir"
	}

	*lxcConfig += "lxc.mount.entry = " +
		escapePath(fileAbs) +
		" " + targetDir + escapePath(fileBase) +
		" none bind,create=" + pType + " 0 0\n"
}

func createHardLink(file, link string) error {
	cmd := exec.Command("busybox", "cp", "-al", file, link)
	err := cmd.Run()
	if err != nil {
		return err
	}

	cmd = exec.Command("chmod", "a+rX", link)
	err = cmd.Run()
	if err != nil {
		return err
	}

	return err
}

func escapePath(path string) string {
	r := strings.NewReplacer(
		" ", "\\040",
		"\t", "\\011",
		"\n", "\\012",
		"\\", "\\\\")
	return r.Replace(path)
}

func makeLxcConfig(cnt, ip string, netEnable bool) string {
	lxcConfig := strings.Replace(cfgTemplate, "<dir>", "/usr/cnt/"+cnt, -1)

	switch cnt {
	case "mpv":
		lxcConfig += cfgDri + cfgSnd
	case "fim":
		lxcConfig += cfgFb
	case "fbpdf":
		lxcConfig += cfgFb
	case "w3m":
		lxcConfig += cfgDri + cfgFb + cfgSnd
		netEnable = true
	case "mutt":
		lxcConfig += cfgFb
		netEnable = true
	case "xorg-server":
		lxcConfig += cfgDri + cfgSnd + cfgInput +
			cfgTty0 + cfgTty4 + cfgUdev
		netEnable = true
	case "dosbox-x":
		lxcConfig += cfgFb + cfgDri + cfgSnd + cfgInput +
			cfgTty0 + cfgTty4 + cfgUdev + cfgSdl
	case "dosbox-staging":
		lxcConfig += cfgFb + cfgDri + cfgSnd + cfgInput +
			cfgUdev + cfgSdl
	case "mednafen":
		lxcConfig += cfgFb + cfgDri + cfgSnd + cfgInput +
			cfgUdev + cfgSdl
	case "exiftool":
		lxcConfig = lxcConfig
	case "vim":
		lxcConfig = lxcConfig
	default:
		errExit(errors.New(""), "container not defined")
	}

	if cnt == "xorg-server" || cnt == "dosbox-x" {
		lxcConfig = strings.Replace(lxcConfig, "<max_pty>", "16", -1)
	} else {
		lxcConfig = strings.Replace(lxcConfig, "<max_pty>", "1", -1)
	}

	if netEnable {
		lxcConfig += cfgNet
		lxcConfig = strings.Replace(lxcConfig, "<ip>", ip, -1)
	} else {
		lxcConfig += cfgNetEmpty
	}

	return lxcConfig
}

func errExit(err error, msg string) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "\n  error: "+msg)
		fmt.Fprintf(os.Stderr, "  %s\n", err)
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

var cfgTemplate = `
lxc.uts.name = xx
lxc.arch = x86_64

lxc.idmap = u 0 100000 65536
lxc.idmap = g 0 100000 65536

lxc.tty.max = 1
lxc.pty.max = <max_pty>

lc.signal.halt = SIGUSR1
lxc.signal.reboot = SIGTERM

lxc.rootfs.path = dir:<dir>
lxc.mount.auto = cgroup:ro proc:rw sys:ro
lxc.autodev = 1

lxc.environment = TERM=linux
lxc.environment = HOME=/home
lxc.environment = PATH=/bin:/sbin
lxc.environment = LC_ALL=en_US.utf8

lxc.mount.entry = /dev/tty dev/tty none bind,create=file 0 0
`

var cfgSnd = "lxc.mount.entry = /dev/snd dev/snd none bind,create=dir 0 0\n"
var cfgFb = "lxc.mount.entry = /dev/fb0 dev/fb0 none bind,create=file 0 0\n"
var cfgDri = "lxc.mount.entry = /dev/dri/card0 dev/dri/card0 none bind,create=file 0 0\n"
var cfgInput = "lxc.mount.entry = /dev/input dev/input none bind,create=dir 0 0\n"
var cfgUdev = "lxc.mount.entry = /run/udev/data run/udev/data none bind,create=dir 0 0\n"
var cfgTty0 = "lxc.mount.entry = /dev/tty0 dev/tty0 none bind,create=file 0 0\n"
var cfgTty4 = "lxc.mount.entry = /dev/tty4 dev/tty4 none bind,create=file 0 0\n"
var cfgSdl = "lxc.environment = SDL_VIDEO_GL_DRIVER=/usr/lib/libGLESv2.so\n"

var cfgShared = "lxc.mount.entry = /mnt/shared mnt/shared none bind,create=dir 0 0\n"

var cfgNetEmpty = "\nlxc.net.0.type = empty\n"
var cfgNet = `
lxc.net.0.type = empty
lxc.net.0.type = veth
lxc.net.0.veth.mode = router
lxc.net.0.ipv4.address = <ip>
lxc.net.0.ipv4.gateway = 10.64.64.1
lxc.net.0.flags = up
lxc.net.0.link = lxc-br0
`
