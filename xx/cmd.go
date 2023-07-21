package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"

	fp "path/filepath"
	str "strings"
)

func (r *runT) execStep(step string, p pkgT, pc pkgCfgT) {
	var command string
	pwd := pc.steps.buildDir

	pathOut := pc.tmpLogDir + "stdout-" + step + ".log"
	pathErr := pc.tmpLogDir + "stderr-" + step + ".log"

	switch step {
	case "prepare":
		fmt.Println("  preparing...")
		command = pc.steps.prepare
		pwd = pc.tmpDir
	case "configure":
		fmt.Println("  configuring...")
		command = pc.steps.configure
	case "build":
		fmt.Printf("  building %s...\n", p.setVerNewRel)
		command = pc.steps.build
	case "pkg_create":
		fmt.Println("  creating pkg...")
		command = pc.steps.pkg_create
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

	r.instLxcConfig(p, pc)
	cmd := r.prepareCmd(p, pc, step, command, pwd, fOut, fErr)
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
		if str.Contains(pc.steps.configure, "meson") {
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

		// clean the new pkg dir on build or pkg_create error
		remNewPkg(p)

		errExit(err, "can't execute command; stderr dump:\n\n"+stderr)
	}
}

func (r *runT) prepareCmd(p pkgT, pc pkgCfgT, step, command, pwd string, fOut, fErr *os.File) *exec.Cmd {

	shCmd := "lxc-execute"
	shCmdP := []string{"-n", "xx", "-P", "/tmp/xx/../"}
	for _, s := range pc.steps.env {
		envVar := str.Split(s, "=")[0]
		shCmdP = append(shCmdP, "-s")
		shCmdP = append(shCmdP, "lxc.environment="+envVar)
	}
	shCmdP = append(shCmdP, []string{"--", "/home/xx/bin/bash", "-c",
		"cd " + pwd + " && " + command}...)

	if pc.crossBuild {
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
	cmd.Env = pc.steps.env
	cmd.Dir = pwd
	logCmd(p, pc, cmd, step)

	return cmd
}

func (r *runT) instLxcConfig(p pkgT, pc pkgCfgT) {
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

	replMap := r.setReplMap(p, pc, pc.src, pc.steps)
	for k, v := range replMap {
		if k == "<root_dir>" && pc.src.srcType == "alpine" {
			v = pc.steps.buildDir + "/rootfs"
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
	config += "lxc.mount.entry = /dev/shm dev/shm none bind,optional,create=dir 0 0\n"

	if fileExists(r.rootDir + "/mnt/xx/boot") {
		config += "lxc.mount.entry = /mnt/xx mnt/xx none bind 0 0\n"
		config += "lxc.mount.entry = /mnt/xx/boot mnt/xx/boot none bind 0 0"
	}

	for _, envVar := range pc.steps.env {
		config += "lxc.environment = " + envVar + "\n"
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
		errExit(err)

		return result.Filesystems[0].Source
	}
	return ""
}

func remNewPkg(p pkgT) {
	errRem := os.RemoveAll(p.newPkgDir)
	if errRem != nil {
		fmt.Fprintln(os.Stderr, "error: can't remove ", errRem)
	}
}

func logCmd(p pkgT, pc pkgCfgT, cmd *exec.Cmd, step string) {
	path := fp.Join(pc.tmpLogDir, "cmd.log")

	fOpts := os.O_CREATE | os.O_APPEND | os.O_WRONLY
	fd, err := os.OpenFile(path, fOpts, 0644)
	errExit(err, "can't create cmd log file")
	defer fd.Close()

	cmdStr := fmt.Sprintf("%+v", cmd)
	cmdStr = str.Replace(cmdStr, " -- ", " -- \n\n", -1)
	cmdStr = str.Replace(cmdStr, "&& ", "&& \n\n", -1)

	envStr := str.Join(pc.steps.env, "\n")

	fmt.Fprintf(fd, "[ %s ]\n\n%+s\n\n%s\n\n\n", step, envStr, cmdStr)
}
