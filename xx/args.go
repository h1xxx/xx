package main

import (
	"os"
	"strconv"

	fp "path/filepath"
	str "strings"
)

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

	errExit(nil, "unrecognized action\n"+
		"  first parameter must be one of: "+
		"(b)uild, (d)iff, (i)nstall, (r)emove, "+
		"(u)pdate, (s)ource, (c)heck, i(n)fo")

	return "error"
}

func (r *runT) parseBuildArgs(args []string) {
	if len(os.Args) < 4 {
		errExit(nil, "missing action target (set file or a program)")
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

		default:
			errExit(nil, "unknown argument:", arg)
		}
	}

	r.checkTarget()
}

func (r *runT) parseInstallArgs(args []string) {
	if len(os.Args) < 4 {
		errExit(nil, "missing action target (set file or a program)")
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

		case "-P", "--Perms":
			r.toInstPerms = true

		case "-C", "--Config":
			r.toInstSysCfg = true

		default:
			errExit(nil, "unknown argument:", arg)
		}
	}

	r.checkTarget()

	if r.sysCfgDir != "" && !fileExists(r.sysCfgDir) {
		errExit(nil, "dir doesn't exist:", r.sysCfgDir)
	}

	if r.toInstPerms && r.sysCfgDir == "" {
		errExit(nil, "missing system config dir")
	}

	if r.toInstSysCfg && r.sysCfgDir == "" {
		errExit(nil, "missing system config dir")
	}

	if r.rootDir == "" {
		errExit(nil, "root dir not defined")
	}

	if r.rootDir[0] != '/' {
		errExit(nil, "root dir must be an absolute path")
	}

	if !fileExists(r.rootDir) {
		errExit(nil, "root dir doesn't exist")
	}
}

func (r *runT) parseSourceArgs(args []string) {
	if len(os.Args) < 4 {
		errExit(nil, "missing action target (set file or a program)")
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

		default:
			errExit(nil, "unknown argument:", arg)
		}
	}

	r.checkTarget()
}

func (r *runT) parseDiffArgs(args []string) {
	if len(os.Args) < 4 {
		errExit(nil, "missing action target (set file or a program)")
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

		default:
			errExit(nil, "unknown argument:", arg)
		}
	}

	if r.diffHours == 0 {
		r.diffHours = 24
	}
}

func (r *runT) parseInfoArgs(args []string) {
	if len(os.Args) < 4 {
		errExit(nil, "missing action target (set file or a program)")
	}

	var shift bool

	for i, arg := range args {
		if shift || arg == "" || i < 2 {
			shift = false
			continue
		}

		switch arg {
		case "-a", "--all":
			r.infoDeps = true

		case "-i", "--integrity":
			r.infoInteg = true

		case "-t", "--target":
			r.actionTarget, shift = getNextArg(args[i:])

		default:
			errExit(nil, "unknown argument:", arg)
		}
	}

	r.checkTarget()
}

func (r *runT) parseUpdateArgs(args []string) {
	if len(os.Args) < 4 {
		errExit(nil, "missing action target (set file or a program)")
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

		default:
			errExit(nil, "unknown argument:", arg)
		}
	}

	r.checkTarget()
}

func (r *runT) checkTarget() {
	if r.actionTarget == "" {
		errExit(nil, "target not defined")
	}

	if fileExists(fp.Join("/home/xx/prog", r.actionTarget)) {
		if r.fixedSet == "" || r.fixedVer == "" {
			errExit(nil, "no version or pkg set defined")
		}

		r.targetIsSinglePkg = true
		return
	}

	if fileExists(r.actionTarget) && str.HasSuffix(r.actionTarget, ".xx") {
		return
	}

	errExit(nil, "not an action target:", r.actionTarget)
}

func getNextArg(args []string) (string, bool) {
	if len(args) < 2 || args[1][0] == '-' {
		errExit(nil, "missing argument after:", args[0])
	}

	return args[1], true
}

/*
func argsCheck(args argsT) {
	// todo: errExit(nil, "no xx file or pkg name provided")

	// checks for when the final arg is a pkg env file
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

	// todo: add checks per action
	//if *args.rootDir == "" && (args.action == "install" || args.action == "i") {
	//	errExit(nil, "root dir argument (-r) missing")
	//}

	// args check: root dir parameter must be an absolute path
}
*/
