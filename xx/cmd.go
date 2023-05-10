package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"

	fp "path/filepath"
	str "strings"
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
		if str.Contains(pkgC.steps.configure, "meson") {
			fd, _ = os.Open(pathOut)
			defer fd.Close()
			print := false
			input = bufio.NewScanner(fd)
			for input.Scan() {
				line := str.ToLower(input.Text())
				if str.Contains(line, "error") ||
					str.Contains(line, "failed") {
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
		envVar := str.Split(s, "=")[0]
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
		isMapLine := str.HasPrefix(line, "lxc.idmap =")
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
			str.Trim(dev, "/") +
			" none bind,create=file 0 0\n" +
			"lxc.mount.entry = " + devBoot + " " +
			str.Trim(devBoot, "/") +
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

	_, err = io.Copy(fd, str.NewReader(config))
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
	cmdStr = str.Replace(cmdStr, " -- ", " -- \n\n", -1)
	cmdStr = str.Replace(cmdStr, "&& ", "&& \n\n", -1)

	envStr := str.Join(pkgC.steps.env, "\n")

	fmt.Fprintf(fd, "[ %s ]\n\n%+s\n\n%s\n\n\n", step, envStr, cmdStr)
}
