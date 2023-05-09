package main

import (
	"bufio"
	"crypto/sha256"
	"debug/elf"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	fp "path/filepath"
	"sort"
	"strings"
)

func (r *runT) execStep(step string, pkg pkgT, pkgC pkgCfgT) {
	var command string
	pwd := pkgC.steps.buildDir

	pathOut := pkgC.tmpLogDir + "stdout-" + step + ".log"
	pathErr := pkgC.tmpLogDir + "stderr-" + step + ".log"

	switch step {
	case "prepare":
		fmt.Println("  preparing...")
		command = pkgC.steps.prepare
		pwd = pkgC.tmpDir
	case "configure":
		fmt.Println("  configuring...")
		command = pkgC.steps.configure
	case "build":
		fmt.Printf("  building %s...\n", pkg.setVerNewRel)
		command = pkgC.steps.build
	case "pkg_create":
		fmt.Println("  creating pkg...")
		command = pkgC.steps.pkg_create
	}

	if command == "" {
		return
	}

	fOut, err := os.Create(pathOut)
	errExit(err, "can't create log file")
	defer fOut.Close()

	fErr, err := os.Create(pathErr)
	errExit(err, "can't create log file")
	defer fErr.Close()

	r.instLxcConfig(pkg, pkgC)
	cmd := r.prepareCmd(pkg, pkgC, step, command, pwd, fOut, fErr)
	err = cmd.Run()
	if err != nil {
		// print error log when a step fails
		stderr := ""
		fd, _ := os.Open(pathErr)
		defer fd.Close()
		input := bufio.NewScanner(fd)
		for input.Scan() {
			stderr += input.Text() + "\n"
		}

		// meson for some reason prints errors to stdout
		if strings.Contains(pkgC.steps.configure, "meson") {
			fd, _ = os.Open(pathOut)
			defer fd.Close()
			print := false
			input = bufio.NewScanner(fd)
			for input.Scan() {
				line := strings.ToLower(input.Text())
				if strings.Contains(line, "error") ||
					strings.Contains(line, "failed") {
					print = true
				}
				if print {
					stderr += input.Text() + "\n"
				}
			}
		}

		// clean the new pkg dir on pkg_create error
		if step == "pkg_create" {
			remNewPkg(pkg, errors.New(""))
		}
		errExit(err, "can't execute command; stderr dump:\n\n"+stderr)
	}
}

func (r *runT) prepareCmd(pkg pkgT, pkgC pkgCfgT, step, command, pwd string, fOut, fErr *os.File) *exec.Cmd {

	shCmd := "lxc-execute"
	shCmdP := []string{"-n", "xx", "-P", "/tmp/xx/../"}
	for _, s := range pkgC.steps.env {
		envVar := strings.Split(s, "=")[0]
		shCmdP = append(shCmdP, "-s")
		shCmdP = append(shCmdP, "lxc.environment="+envVar)
	}
	shCmdP = append(shCmdP, []string{"--", "/home/xx/bin/bash", "-c",
		"cd " + pwd + " && " + command}...)

	if pkgC.crossBuild {
		shCmd = "/bin/sh"
		shCmdP = []string{"-c", command}
	}

	if r.rootDir == "/" {
		shCmd = "/home/xx/bin/bash"
		shCmdP = []string{"-c", command}
	}

	cmd := exec.Command(shCmd, shCmdP...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = fOut
	cmd.Stderr = fErr
	cmd.Env = pkgC.steps.env
	cmd.Dir = pwd
	logCmd(pkg, pkgC, cmd, step)

	return cmd
}

func (r *runT) instLxcConfig(pkg pkgT, pkgC pkgCfgT) {
	var config string

	user, err := user.Current()
	errExit(err, "can't get user info")

	fd, err := os.Open("/home/xx/misc/lxc_config")
	errExit(err, "can't read lxc config file")
	input := bufio.NewScanner(fd)
	for input.Scan() {
		line := input.Text()
		isMapLine := strings.HasPrefix(line, "lxc.idmap =")
		if isMapLine && user.Username == "root" {
			continue
		}
		config += line + "\n"
	}
	fd.Close()

	replMap := r.setReplMap(pkg, pkgC, pkgC.src, pkgC.steps)
	for k, v := range replMap {
		if k == "<root_dir>" && pkgC.src.srcType == "alpine" {
			v = pkgC.steps.buildDir + "/rootfs"
		}
		repl(&config, k, v)
	}
	repl(&config, "<user_id>", user.Uid)
	repl(&config, "<group_id>", user.Gid)

	dev := getMountDev(r.rootDir)
	devBoot := getMountDev(r.rootDir + "/boot")

	// add entries to pass devices from target dir mountpoints
	if dev != "" && devBoot != "" && r.rootDir != "" {
		devCfg := "lxc.mount.entry = " + dev + " " +
			strings.Trim(dev, "/") +
			" none bind,create=file 0 0\n" +
			"lxc.mount.entry = " + devBoot + " " +
			strings.Trim(devBoot, "/") +
			" none bind,create=file 0 0\n"
		config += devCfg
	}

	if fileExists(r.rootDir + "/mnt/xx/boot") {
		config += "lxc.mount.entry = /mnt/xx mnt/xx none bind 0 0\n"
		config += "lxc.mount.entry = /mnt/xx/boot mnt/xx/boot none bind 0 0"
	}

	err = os.MkdirAll("/tmp/xx/build", 0700)
	errExit(err, "couldn't create '/tmp/xx/' dir")

	fd, err = os.Create("/tmp/xx/config")
	errExit(err, "can't create lxc config file")
	defer fd.Close()

	_, err = io.Copy(fd, strings.NewReader(config))
	errExit(err, "can't write to lxc config file")
}

func getMountDev(mountPoint string) string {
	type fsT struct {
		Target  string `json:"target"`
		Source  string `json:"source"`
		Fstype  string `json:"fstype"`
		Options string `json:"options"`
	}
	type findMntT struct {
		Filesystems []fsT `json:"filesystems"`
	}

	cmd := exec.Command("/bin/findmnt", "-J", mountPoint)
	out, err := cmd.Output()
	if err == nil {
		var result findMntT
		err := json.Unmarshal(out, &result)
		if err != nil {
			os.Exit(1)
		}
		return result.Filesystems[0].Source
	}
	return ""
}

func remNewPkg(pkg pkgT, err error) {
	if err != nil {
		errRem := os.RemoveAll(pkg.newPkgDir)
		if errRem != nil {
			fmt.Fprintln(os.Stderr, "error: can't remove ", errRem)
		}
	}
}

func logCmd(pkg pkgT, pkgC pkgCfgT, cmd *exec.Cmd, step string) {
	path := fp.Join(pkgC.tmpLogDir, "cmd.log")

	fOpts := os.O_CREATE | os.O_APPEND | os.O_WRONLY
	fd, err := os.OpenFile(path, fOpts, 0644)
	errExit(err, "can't create cmd log file")
	defer fd.Close()

	cmdStr := fmt.Sprintf("%+v", cmd)
	cmdStr = strings.Replace(cmdStr, " -- ", " -- \n\n", -1)
	cmdStr = strings.Replace(cmdStr, "&& ", "&& \n\n", -1)

	envStr := strings.Join(pkgC.steps.env, "\n")

	fmt.Fprintf(fd, "[ %s ]\n\n%+s\n\n%s\n\n\n", step, envStr, cmdStr)
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
		file = strings.TrimPrefix(file, pkg.newPkgDir)
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
		if strings.HasSuffix(lib, ".c32") {
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
		!strings.Contains(pkgC.steps.configure, "meson"):

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
		if strings.HasSuffix(file, ".la") {
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
		if strings.Contains(file, "/go/pkg/") && strings.HasSuffix(file, ".a") {
			continue
		}

		if strings.HasPrefix(file, pkg.newPkgDir+"/lib/") {
			lib = true
		} else if strings.HasPrefix(file, pkg.newPkgDir+"/usr/lib/") {
			usrLib = true
		}

		binDirs := []string{"/bin/", "/sbin/", "/usr/bin/",
			"/usr/sbin/", "/usr/libexec/", "/tools/bin"}
		for _, dir := range binDirs {
			if strings.HasPrefix(file, pkg.newPkgDir+dir) {
				bin = true
				break
			}
		}

		if lib && ext == ".a" {
			runStrip("--strip-debug", file)
		} else if (usrLib || lib) && strings.HasPrefix(ext, ".so") {
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

func pressAKey() {
	fmt.Println("\n  new package dir is now going to be removed. " +
		"press any key to continue...")
	var b []byte = make([]byte, 1)
	os.Stdin.Read(b)
}
