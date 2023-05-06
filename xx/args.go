package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	fp "path/filepath"
	str "strings"
)

type argsT struct {
	rootDir *string
	set     *string
	Ver     *string
	hours   *string
	c       *string
	b       *bool
	B       *bool
	d       *bool
	P       *bool
	C       *bool
	i       *bool
	v       *bool
}

func parseArgs() runT {
	var r runT

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	r.action = getAction(os.Args)

	switch r.action {
	case "build":
		r.parseBuildArgs(os.Args)

	case "install":
		r.parseInstallArgs(os.Args)

	case "source":
		r.parseSourceArgs(os.Args)

	case "diff":
		r.parseDiffArgs(os.Args)

	case "info":
		r.parseInfoArgs(os.Args)

	case "update":
		r.parseUpdateArgs(os.Args)
	}

	return r

	/*
		args.d = flag.Bool("d", false, "s: download source; i: show deps")
		args.P = flag.Bool("P", false, "i: only set permissions for root dir")
		args.C = flag.Bool("C", false, "i: only install configs for root dir")
		args.i = flag.Bool("i", false, "n: verify system integrity")

		if *args.set != "xx_tools_cross" {
			testTools()
		}
	*/
}

func (r *runT) parseBuildArgs(args []string) {
	if len(os.Args) < 4 {
		msg := "missing action target (set file or a program)"
		errExit(fmt.Errorf(msg), "")
	}

	var shift bool

	for i, arg := range args {
		if shift || arg == "" || i < 2 {
			shift = false
			continue
		}

		switch arg {
		case "-f", "--force":
			r.forceAll = true

		case "-t", "--target":
			r.actionTarget, shift = getNextArg(args[i:])

		case "-s", "--pkg-set":
			r.fixedSet, shift = getNextArg(args[i:])

		case "-V", "--pkg-version":
			r.fixedVer, shift = getNextArg(args[i:])
		}
	}

	r.checkTarget()
}

func (r *runT) parseInstallArgs(args []string) {
	if len(os.Args) < 4 {
		msg := "missing action target (set file or a program)"
		errExit(fmt.Errorf(msg), "")
	}

	var shift bool

	for i, arg := range args {
		if shift || arg == "" || i < 2 {
			shift = false
			continue
		}

		switch arg {
		case "-f", "--force":
			r.forceAll = true

		case "-t", "--target":
			r.actionTarget, shift = getNextArg(args[i:])

		case "-s", "--pkg-set":
			r.fixedSet, shift = getNextArg(args[i:])

		case "-V", "--pkg-version":
			r.fixedVer, shift = getNextArg(args[i:])

		case "-r", "--root-dir":
			r.rootDir, shift = getNextArg(args[i:])

		case "-c", "--config-dir":
			r.sysCfgDir, shift = getNextArg(args[i:])
		}
	}

	r.checkTarget()

	if r.sysCfgDir != "" && !fileExists(r.sysCfgDir) {
		errExit(fmt.Errorf("dir doesn't exist: %s", r.sysCfgDir), "")
	}

	if r.rootDir == "" {
		errExit(fmt.Errorf("root dir not defined"), "")
	}

	if r.rootDir[0] != '/' {
		errExit(fmt.Errorf("root dir must be an absolute path"), "")
	}
}

func (r *runT) parseSourceArgs(args []string) {
	if len(os.Args) < 4 {
		msg := "missing action target (set file or a program)"
		errExit(fmt.Errorf(msg), "")
	}

	var shift bool

	for i, arg := range args {
		if shift || arg == "" || i < 2 {
			shift = false
			continue
		}

		switch arg {
		case "-t", "--target":
			r.actionTarget, shift = getNextArg(args[i:])
		}
	}

	r.checkTarget()
}

func (r *runT) parseDiffArgs(args []string) {
	if len(os.Args) < 4 {
		msg := "missing action target (set file or a program)"
		errExit(fmt.Errorf(msg), "")
	}

	var shift bool

	for i, arg := range args {
		if shift || arg == "" || i < 2 {
			shift = false
			continue
		}

		switch arg {
		case "-b", "--build":
			r.diffBuild = true

		case "-h", "--hours":
			var hoursStr string
			hoursStr, shift = getNextArg(args[i:])

			hoursInt, err := strconv.Atoi(hoursStr)
			msg := "can't convert hours parameter to integer: "
			msg += hoursStr
			errExit(err, msg)

			r.diffHours = int64(hoursInt)
		}
	}

	if r.diffHours == 0 {
		r.diffHours = 24
	}
}

func (r *runT) parseInfoArgs(args []string) {
	if len(os.Args) < 4 {
		msg := "missing action target (set file or a program)"
		errExit(fmt.Errorf(msg), "")
	}

	var shift bool

	for i, arg := range args {
		if shift || arg == "" || i < 2 {
			shift = false
			continue
		}

		switch arg {
		case "-d", "--deps":
			r.infoDeps = true

		case "-i", "--integrity":
			r.infoInteg = true

		case "-t", "--target":
			r.actionTarget, shift = getNextArg(args[i:])
		}
	}

	r.checkTarget()
}

func (r *runT) parseUpdateArgs(args []string) {
	if len(os.Args) < 4 {
		msg := "missing action target (set file or a program)"
		errExit(fmt.Errorf(msg), "")
	}

	var shift bool

	for i, arg := range args {
		if shift || arg == "" || i < 2 {
			shift = false
			continue
		}

		switch arg {
		case "-t", "--target":
			r.actionTarget, shift = getNextArg(args[i:])
		}
	}

	r.checkTarget()
}

func (r *runT) checkTarget() {
	if r.actionTarget == "" {
		errExit(fmt.Errorf("target not defined"), "")
	}

	if fileExists(fp.Join("/home/xx/prog", r.actionTarget)) {
		if r.fixedSet == "" || r.fixedVer == "" {
			msg := "no version or pkg set defined"
			errExit(fmt.Errorf(msg), "")
		}
		return
	}

	if fileExists(r.actionTarget) {
		return
	}

	errExit(fmt.Errorf("no target file '%s'", r.actionTarget), "")
}

func getNextArg(args []string) (string, bool) {
	if len(args) < 2 || args[1][0] == '-' {
		errExit(fmt.Errorf("missing argument after %s", args[0]), "")
	}

	return args[1], true
}

func getAction(args []string) string {
	arg := os.Args[1]

	actions := map[string]string{
		"b": "build",
		"i": "install",
		"s": "source",
		"d": "diff",
		"n": "info",
		"u": "update",
	}

	for abbr, action := range actions {
		if arg == action || arg == abbr {
			return action
		}
	}

	errExit(errors.New(""), "unrecognized action\n"+
		"  first parameter must be one of: "+
		"(b)uild, (d)iff, (i)nstall, (r)emove, "+
		"(u)pdate, (s)ource, (c)heck, i(n)fo")

	return "error"
}

func getRunVars() runT {
	var args argsT
	var r runT

	r.buildEnv = getBuildEnv(r.actionTarget)
	r.baseDir = "/tmp/xx/base"
	r.baseFile = "/home/xx/set/base.xx"
	if r.buildEnv == "base" || r.buildEnv == "musl_base" {
		r.baseEnv = true
	}
	if str.HasPrefix(r.buildEnv, "musl_") {
		r.muslEnv = true
		r.baseDir = "/tmp/xx/musl_base"
		r.baseFile = "/home/xx/set/musl_base.xx"
	}

	if str.HasPrefix(r.buildEnv, "init_") {
		r.isInit = true
	}

	// this file shows that the base pkgs are currently being bootstrapped
	if !r.muslEnv && fileExists("/tmp/xx/base/bootstrap") {
		r.isInit = true
	}

	r.rootDir = getRootDir(args, r.buildEnv)

	if !isPkgString(r.actionTarget) {
		r.setFileName = fp.Base(r.actionTarget)
	}

	if *args.set != "std" {
		r.fixedSet = *args.set
	}

	if *args.Ver != "latest" {
		r.fixedVer = *args.Ver
	}

	t := time.Now()
	fStr := "%d-%.2d-%.2d"
	r.date = fmt.Sprintf(fStr, t.Year(), t.Month(), t.Day())

	r.sysCfgDir = fp.Clean(*args.c)

	r.toInstPerms = *args.P
	r.toInstSysCfg = *args.C
	r.verbose = *args.v

	r.infoDeps = *args.d
	r.infoInteg = *args.i

	//if !isPkgString(r.actionTarget) {
	//	r.runDeps = r.readDeps("run")
	//	r.buildDeps = r.readDeps("build")
	//}

	return r
}

func getRootDir(args argsT, buildEnv string) string {
	var rootDir string

	if *args.rootDir == "" {
		rootDir = fp.Join("/tmp/xx/", buildEnv)
	} else if str.Contains(*args.rootDir, ":/") {
		// just clean up the path when the host is remote
		split := str.Split(*args.rootDir, ":")
		host := split[0]
		path := split[1]
		path = fp.Clean(path)
		rootDir = host + ":" + path
	} else {
		rootDir = fp.Clean(*args.rootDir)
	}

	return rootDir
}

func argsCheck(args argsT) {
	// todo: errExit(errors.New(""), "no xx file or pkg name provided")

	// checks for when the final arg is a pkg env file
	/*
		if !isPkgString(args.actionTarget) {
			path := args.actionTarget
			stat, err := os.Stat(path)
			errExit(err, "can't stat "+path)
			if stat.IsDir() || !str.HasSuffix(path, ".xx") {
				errExit(err, msg)
			}
		}

		// checks for when the final arg is a pkg name
		if isPkgString(args.actionTarget) {
			path := "/home/xx/prog/" + args.actionTarget
			stat, err := os.Stat(path)
			if err != nil || !stat.IsDir() {
				errExit(err, msg)
			}
		}
	*/

	// todo: add checks per action
	//if *args.rootDir == "" && (args.action == "install" || args.action == "i") {
	//	errExit(errors.New(""), "root dir argument (-r) missing")
	//}

	// args check: root dir parameter must be an absolute path
}
