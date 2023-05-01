package main

import (
	"fmt"
	"os"

	fp "path/filepath"
	str "strings"
)

func (r *runT) makeLxcConfig() {
	r.lxcConfig = fmt.Sprintf(cfgTemplate, r.dirs.cnt)

	if r.cntConf.tty0 {
		r.lxcConfig += addMount("/dev/tty0", "file")
	}

	if r.cntConf.tty4 {
		r.lxcConfig += addMount("/dev/tty4", "file")
	}

	if r.cntConf.fb {
		r.lxcConfig += addMount("/dev/fb0", "file")
	}

	if r.cntConf.dri {
		r.lxcConfig += addMount("/dev/dri", "dir")
	}

	if r.cntConf.snd {
		r.lxcConfig += addMount("/dev/snd", "dir")
	}

	if r.cntConf.input {
		r.lxcConfig += addMount("/dev/input", "dir")
	}

	if r.cntConf.udev {
		r.lxcConfig += addMount("/run/udev", "dir")
	}

	if r.cntConf.sdl {
		r.lxcConfig += addEnv("SDL_VIDEO_GL_DRIVER=/lib/libGLESv2.so")
	}

	if r.cntConf.shared {
		r.lxcConfig += addMount("/mnt/shared", "dir")
	}

	if r.cntConf.net {
		r.lxcConfig += fmt.Sprintf(cfgNet, r.cntIP)
	} else {
		r.lxcConfig += cfgNetEmpty
	}

	// add bind mounts for files and dirs in command arguments
	for _, t := range r.bindTargets {
		r.addBind(t)
	}

	// bind /cnt/home/<cnt> to /home/cnt in a container
	r.bindHome()

	r.lxcConfig += "\nlxc.execute.cmd = /cmd"
}

func (r *runT) writeLxcConfig() {
	f := fp.Join(r.dirs.cnt, "config")

	fd, err := os.OpenFile(f, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	errExit(err)

	_, err = fmt.Fprintf(fd, r.lxcConfig)
	errExit(err)
}

func addMount(path, pType string) string {
	formatS := "lxc.mount.entry = %s %s none bind,create=%s 0 0\n"

	return fmt.Sprintf(formatS, path, str.TrimPrefix(path, "/"), pType)
}

func addEnv(envVar string) string {
	return fmt.Sprintf("lxc.environment = %s\n", envVar)
}

func (r *runT) addBind(path string) {
	path = getNoSpacePath(path)
	bindPath := fp.Join("bind", path)

	bindFullDir := fp.Join(r.dirs.bind, fp.Dir(path))

	if r.debug {
		pr("creating %s...", bindFullDir)
	}

	// todo: use 770, add cnt user to x group
	err := os.MkdirAll(bindFullDir, 0777)
	errExit(err)

	pType := "file"
	bindType := "bind"
	if isDir(path) {
		pType = "dir"
		bindType = "rbind"
	}

	formatS := "lxc.mount.entry = %s %s none %s,create=%s 0 0\n"
	r.lxcConfig += fmt.Sprintf(formatS, path, bindPath, bindType, pType)
}

// finds last subpath that doesn't have spaces
func getNoSpacePath(path string) string {
	for str.Contains(path, " ") {
		path = fp.Dir(path)
	}

	return path
}

func (r *runT) bindHome() {
	formatS := "lxc.mount.entry = %s home/cnt none bind,create=dir 0 0\n"
	r.lxcConfig += fmt.Sprintf(formatS, r.dirs.home)
}

func escapePath(path string) string {
	r := str.NewReplacer(" ", "__")

	/*
		todo: to be used once lxc config can support \040 as a space
		in lxc.mount.entry

		r := str.NewReplacer(
			" ", "\\040",
			"\t", "\\011",
			"\n", "\\012",
			"\\", "\\\\")
	*/

	return r.Replace(path)
}
