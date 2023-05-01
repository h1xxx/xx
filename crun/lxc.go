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

func (r *runT) addBind(t string) {
	bindPath := fp.Join("bind", t)
	bindFullDir := fp.Join(r.dirs.bind, fp.Dir(t))

	// todo: try 770, add cnt user to x group
	if r.debug {
		pr("creating %s...", bindFullDir)
	}

	// todo: use 770, add cnt user to x group
	err := os.MkdirAll(bindFullDir, 0777)
	errExit(err)

	bindPath = escapePath(bindPath)

	pType := "file"
	if isDir(t) {
		pType = "dir"
	}

	formatS := "lxc.mount.entry = %s %s none bind,create=%s 0 0\n"
	r.lxcConfig += fmt.Sprintf(formatS, escapePath(t), bindPath, pType)
}

func (r *runT) bindHome() {
	formatS := "lxc.mount.entry = %s home/cnt none bind,create=dir 0 0\n"
	r.lxcConfig += fmt.Sprintf(formatS, r.dirs.home)
}

func escapePath(path string) string {
	r := str.NewReplacer(
		" ", "\\040",
		"\t", "\\011",
		"\n", "\\012",
		"\\", "\\\\")
	return r.Replace(path)
}
