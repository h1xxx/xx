package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	fp "path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// worldT stores information on all files and packages installed in the env;
//
// files	map files -> pkgs
// fileHash	map file -> hash
// pkgFiles	map pkgs -> []files
// pkgs		map pkgs -> true
type worldT struct {
	files    map[string]pkgT
	fileHash map[string]string
	pkgFiles map[pkgT][]string
	pkgs     map[pkgT]bool
}

type argsT struct {
	action       string
	actionTarget string
	set          *string
	Ver          *string
	rootDir      *string
	hours        *string
	forceAll     *bool
	c            *string
	b            *bool
	B            *bool
	d            *bool
	P            *bool
	C            *bool
	i            *bool
	v            *bool
}

// genCfgT stores general configuration for the command
//
// rootDir	root system dir:		/, /mnt/xx, /tmp/xx/media
// sysCfgDir	dir with system config:		/home/xx/conf/<machine>
// setFileName	pkg env definition file:	[empty], base.xx, media.xx
// buildEnv	name of the build environment:	base, bootstrap, net, media
// actionTarget	package name or pkg env file:	sys/lvm2, base.xx, media.xx
// action	action to perform:		build, install, update, diff
// date		current calendar date:		2002-05-13
//
// forceAll	force action on all packages:	false, true
// fixedSet	fixed pkg set for all packages:	[empty], std, bootstrap
// fixedVer	fixed pkg ver for all packages:	[empty], latest, 2.35
//
// instPerms	only install sys permissions:	false, true
// instSysCfg	only install system config:	false, true
// verbose	verbose messages:		false, true
//
// diffBuild	show diff to previous build:	false, true
// diffHours	show diff within X last hours:	24, 30, 48
//
// infoDeps	show info on dependencies	false, true
// infoInteg	check system integrity		false, true
type genCfgT struct {
	rootDir      string
	sysCfgDir    string
	setFileName  string
	buildEnv     string
	actionTarget string
	action       string
	actionOpts   string
	date         string

	forceAll bool
	fixedSet string
	fixedVer string

	instPerms  bool
	instSysCfg bool
	verbose    bool

	diffBuild bool
	diffHours int64

	infoDeps  bool
	infoInteg bool

	buildDeps map[pkgT][]pkgT
	runDeps   map[pkgT][]pkgT
}

// pkgT stores common variables to uniquely identify a package
//
// name		package name (format <category/program>):	sys/lvm2
// categ	program category: 				sys
// prog		program name:					lvm2
// set		package set:					std, bootstrap_cross
// ver		resolved program version, not 'latest':		2.35
// verShort	short ver; in case of git repo, commit hash:	2, 4d21a91b
// rel		current pkg release (build) version in hex:	01, 0a, 1e
// setVerRel	combination of <pkg set>-<ver>-<pkg rel>:	std-2.35-01
//
// prevRel	previous pkg release (build) version in hex:	00, 09, 1d
// newRel	next pkg release (build) version in hex:	02, 0b, 1f
// setVerPrevRel combination of <pkg set>-<ver>-<prev pkg rel>:	std-2.35-00
// setVerNewRel  combination of <pkg set>-<ver>-<new pkg rel>:	std-2.35-02
//
// progDir	progam dir:			/home/xx/sys/lvm2
// pkgDir	dir with current release:	<prog_dir>/pkg/std_1-2.35-09
// newPkgDir	dir with new release:		<prog_dir>/pkg/std_1-2.35-0a
// prevPkgDir	dir with new release:		<prog_dir>/pkg/std_1-2.35-08
// cfgDir	dir with pkg config:		<prog_dir>/cfg/std-2.35
//
type pkgT struct {
	name      string
	categ     string
	prog      string
	set       string
	ver       string
	verShort  string
	rel       string
	setVerRel string

	prevRel       string
	newRel        string
	setVerPrevRel string
	setVerNewRel  string

	progDir    string
	pkgDir     string
	newPkgDir  string
	prevPkgDir string
	cfgDir     string
}

// pkgCfgT stores configuration of the package and build environment
//
// instDir	dir to install a pkg to:	<root_dir>, /cnt/rootfs/<prog>
// tmpDir	tmp build dir:		/tmp/xx/build/lvm2-2.35_build-00
// tmpLogDir	tmp log dir:		/tmp/xx/build/lvm2-2.35_build-00/log
//
// force	force building or installation of the package
// cnt		package is to be installed in a container
// cntPkg	root package for the container
// cntProg	name of the pkg container in /cnt/rootfs/
// crossBuild	program is compiled with host tools
// subPkg	program set is a subpkg created during main build
//
// src		struct with source code location and type
// steps	struct with shell commands to execute
// cfgFiles	locations of config files for the pkg
//
// libDeps	list of pkgs with shared libraries needed to run the program
// runDeps	list of pkgs needed to run the program excl. shared libraries
// buildDeps	list of pkgs needed to build the program
// allRunDeps	list of all pkgs needed to run the program
type pkgCfgT struct {
	instDir   string
	tmpDir    string
	tmpLogDir string

	force      bool
	cnt        bool
	cntPkg     pkgT
	cntProg    string
	crossBuild bool
	subPkg     bool

	src      srcT
	steps    stepsT
	cfgFiles map[string]string

	libDeps    []pkgT
	runDeps    []pkgT
	buildDeps  []pkgT
	allRunDeps []pkgT
}

// srcT stores information from ini file on source code location and type
// url		url to source code:	https://ftp.gnu.org/lvm2-2.35.tar.xz
// file		downloaded file		<src_dir>/lvm2-2.35.tar.xz
// dirName	dir in tar archive:	lvm2-2.35
// srcType	type of source code:	tar, git, files, go-mod, alpine
type srcT struct {
	url     string
	file    string
	dirName string
	srcType string
}

// stepsT stores shell commands from ini file to be executed in build steps
// env		environment variables to use during build
// buildDir	root build dir:	/tmp/xx/build/lvm2-2.35_build-00/lvm2-2.35
//
// subPkgs	sub packages to create with files to move from the main pkg
type stepsT struct {
	env      []string
	buildDir string

	prepare    string
	configure  string
	build      string
	pkg_create string

	subPkgs map[string][]string
}

// reT contains regexes for various checks and subsitutions
//
// wSpaces		matches multiple whitespaces
// pkgName		matches a correct package name
type reT struct {
	wSpaces *regexp.Regexp
	pkgName *regexp.Regexp
}

func main() {
	var args argsT

	args.set = flag.String("s", "std", "b,i: pkg config set to build")
	args.Ver = flag.String("V", "latest", "b,i: program version")
	args.forceAll = flag.Bool("f", false, "b,i: force action")
	args.rootDir = flag.String("r", "", "i: root dir for xx install")
	args.hours = flag.String("h", "24", "time horizon to search for diffs")
	args.c = flag.String("c", "", "i: cfg dir to apply on install")
	args.d = flag.Bool("d", false, "s: download source; i: show deps")
	args.b = flag.Bool("b", false, "d: show diff against previous build")
	args.P = flag.Bool("P", false, "i: only set permissions for root dir")
	args.C = flag.Bool("C", false, "i: only install configs for root dir")
	args.i = flag.Bool("i", false, "n: verify system integrity")
	args.v = flag.Bool("v", false, "all: verbose messages")

	// todo: move args check to a central place
	if len(os.Args) < 3 {
		printUsage()
		os.Exit(1)
	}

	args.action = os.Args[1]
	lastArgPos := len(os.Args) - 1
	args.actionTarget = os.Args[lastArgPos]

	os.Args = append(os.Args[:1], os.Args[2:lastArgPos]...)
	flag.Usage = printUsage
	flag.Parse()

	if *args.set != "xx_tools_cross" {
		testTools(args)
	}

	argsCheck(args)

	genC := getGenCfg(args)
	pkgs, pkgCfgs := getPkgList(genC)
	world := getWorld(genC, pkgCfgs)

	switch {
	case flag.NArg() > 0:
		errExit(errors.New(""), "unrecognized parameter\n"+
			"  first parameter must be one of: "+
			"(b)uild, (d)iff, (i)nstall, (r)emove, "+
			"(u)pdate, (s)ource, (c)heck, i(n)fo")
	case genC.action == "build":
		actionBuild(world, genC, pkgs, pkgCfgs)
	case genC.action == "diff":
		actionDiff(genC, pkgs, pkgCfgs)
	case genC.action == "install":
		actionInst(world, genC, pkgs, pkgCfgs)
	//case genC.action == "remove":
	//	actionInst(pkg, pkgCfgs)
	case genC.action == "update":
		actionUpdate(pkgs, pkgCfgs)
	case genC.action == "source":
		actionSource(genC, pkgs, pkgCfgs)
	case genC.action == "check":
		actionCheck(genC)
	case genC.action == "info":
		actionInfo(genC, pkgs, pkgCfgs)
	case genC.action == "--help" || args.action == "-h" ||
		args.action == "help":
		printUsage()
	default:
		errExit(errors.New(""), "unrecognized action\n"+
			"  first parameter must be one of: "+
			"(b)uild, (d)iff, (i)nstall, (r)emove, "+
			"(u)pdate, (s)ource, (c)heck, i(n)fo")
	}
}

func argsCheck(args argsT) {
	msg := "last argument must be a path to a pkg dir or a definition file"

	// todo: errExit(errors.New(""), "no xx file or pkg name provided")

	// checks for when the final arg is a pkg env file
	if !isPkgString(args.actionTarget) {
		path := args.actionTarget
		stat, err := os.Stat(path)
		if stat.IsDir() || !strings.HasSuffix(path, ".xx") {
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
	if *args.rootDir == "" && (args.action == "install" || args.action == "i") {
		errExit(errors.New(""), "root dir argument (-r) missing")
	}
}

func getWorld(genC genCfgT, pkgCfgs []pkgCfgT) map[string]worldT {
	world := make(map[string]worldT)
	initWorldEntry(world, "/")
	worldPkgs := getWorldPkgs(genC, genC.rootDir)
	if genC.action == "build" && genC.buildEnv != "base" {
		basePkgs := getWorldPkgs(genC, "/tmp/xx/base")
		worldPkgs = append(worldPkgs, basePkgs...)
	}

	for _, pkg := range worldPkgs {
		addPkgToWorldT(world, pkg, "/")
	}

	cntDir := fp.Join(genC.rootDir, "/cnt/rootfs")
	cntList := getCntList(cntDir)
	for _, pkgC := range pkgCfgs {
		if pkgC.cnt && !stringExists(pkgC.cntProg, cntList) {
			cntList = append(cntList, pkgC.cntProg)
		}
	}

	for _, cntProg := range cntList {
		initWorldEntry(world, cntProg)
	}

	for _, cntProg := range cntList {
		cntRootDir := fp.Join(genC.rootDir, "/cnt/rootfs/", cntProg)
		cntWorldPkgs := getWorldPkgs(genC, cntRootDir)

		for _, pkg := range cntWorldPkgs {
			addPkgToWorldT(world, pkg, cntProg)
		}
	}

	return world
}

func initWorldEntry(world map[string]worldT, entry string) {
	world[entry] = worldT{
		files:    make(map[string]pkgT),
		fileHash: make(map[string]string),
		pkgFiles: make(map[pkgT][]string),
		pkgs:     make(map[pkgT]bool),
	}
}

func getGenCfg(args argsT) genCfgT {
	var genC genCfgT

	abbr := make(map[string]string)
	abbr["b"] = "build"
	abbr["i"] = "install"
	abbr["s"] = "source"
	abbr["d"] = "diff"
	abbr["n"] = "info"
	abbr["u"] = "update"

	genC.action = args.action
	if len(genC.action) == 1 {
		genC.action = abbr[genC.action]
	}

	genC.actionTarget = args.actionTarget
	genC.buildEnv = getBuildEnv(genC.actionTarget)
	genC.rootDir = getRootDir(args, genC.buildEnv)

	if !isPkgString(genC.actionTarget) {
		genC.setFileName = filepath.Base(genC.actionTarget)
	}

	if *args.set != "std" {
		genC.fixedSet = *args.set
	}

	if *args.Ver != "latest" {
		genC.fixedVer = *args.Ver
	}

	t := time.Now()
	fStr := "%d-%.2d-%.2d"
	genC.date = fmt.Sprintf(fStr, t.Year(), t.Month(), t.Day())

	genC.sysCfgDir = filepath.Clean(*args.c)
	genC.forceAll = *args.forceAll

	genC.instPerms = *args.P
	genC.instSysCfg = *args.C
	genC.verbose = *args.v

	h, err := strconv.Atoi(*args.hours)
	errExit(err, "can't convert hours parameter to integer: "+*args.hours)
	genC.diffHours = int64(h)
	genC.diffBuild = *args.b

	genC.infoDeps = *args.d
	genC.infoInteg = *args.i

	if !isPkgString(genC.actionTarget) {
		genC.runDeps = readDeps(genC, "run")
		genC.buildDeps = readDeps(genC, "build")
	}

	return genC
}

func getPkgList(genC genCfgT) ([]pkgT, []pkgCfgT) {
	var pkgs []pkgT
	var pkgCfgs []pkgCfgT

	// process a pkg env file
	if genC.setFileName != "" {
		pkgs, pkgCfgs = parsePkgEnvFile(genC.actionTarget, genC)
	}

	// process a single package
	if genC.setFileName == "" {
		pkg := getPkg(genC, genC.actionTarget, "std", "latest")
		pkgC := getPkgCfg(genC, pkg, "")

		pkgs = append(pkgs, pkg)
		pkgCfgs = append(pkgCfgs, pkgC)
	}

	return pkgs, pkgCfgs
}

func getPkg(genC genCfgT, name, pkgSet, ver string) pkgT {
	var pkg pkgT
	fields := strings.Split(name, "/")

	pkg.name = name
	pkg.categ = fields[0]
	pkg.prog = fields[1]

	pkg.set = pkgSet
	if genC.fixedSet != "" && genC.fixedSet != "std" {
		pkg.set = genC.fixedSet
	}

	pkg.progDir = fp.Join("/home/xx/prog", pkg.name)
	pkg.ver = getVer(pkg, ver)
	if genC.fixedVer != "" && genC.fixedVer != "latest" {
		pkg.ver = genC.fixedVer
	}
	pkg.verShort = getVerShort(pkg.ver)

	pkg.rel, pkg.prevRel, pkg.newRel = getPkgRels(pkg)
	pkg = getPkgSetVers(pkg)
	pkg = getPkgDirs(pkg)

	return pkg
}

func getPkgSetVers(pkg pkgT) pkgT {
	setVer := pkg.set + "-" + pkg.ver
	pkg.setVerRel = setVer + "-" + pkg.rel
	pkg.setVerPrevRel = setVer + "-" + pkg.prevRel
	pkg.setVerNewRel = setVer + "-" + pkg.newRel

	return pkg
}

func getPkgDirs(pkg pkgT) pkgT {
	pkg.pkgDir = fp.Join(pkg.progDir, "pkg", pkg.setVerRel)
	pkg.newPkgDir = fp.Join(pkg.progDir, "pkg", pkg.setVerNewRel)
	pkg.prevPkgDir = fp.Join(pkg.progDir, "pkg", pkg.setVerPrevRel)

	cfgDir := fp.Join(pkg.progDir, "cfg", pkg.set+"-"+pkg.ver)
	if fileExists(cfgDir) {
		pkg.cfgDir = cfgDir
	} else {
		pkg.cfgDir = fp.Join(pkg.progDir, "cfg", pkg.set+"-latest")
	}

	return pkg
}

func getPkgCfg(genC genCfgT, pkg pkgT, flags string) pkgCfgT {
	var pkgC pkgCfgT
	pkgC.force, pkgC.cnt = parsePkgFlags(flags, pkg.name)

	if genC.forceAll {
		pkgC.force = true
	}

	if pkgC.cnt {
		pkgC.cntPkg = pkg
		pkgC.cntProg = pkg.prog
		pkgC.instDir = fp.Join(genC.rootDir, "/cnt/rootfs/",
			pkgC.cntProg)
	} else {
		pkgC.instDir = genC.rootDir
	}

	buildPath := "/tmp/xx/build/" + pkg.prog + "-" + pkg.ver + "_build-"
	buildID := getLastRel("/tmp/xx/build/", pkg.prog+"-"+pkg.ver+"_build-")
	if fileExists(buildPath + "00") {
		buildID += 1
	}
	pkgC.tmpDir = buildPath + fmt.Sprintf("%0.2x", buildID)
	pkgC.tmpLogDir = pkgC.tmpDir + "/log/"

	if strings.HasSuffix(pkg.set, "_cross") {
		pkgC.crossBuild = true
	}

	if strings.Contains(pkg.set, "_") && !pkgC.crossBuild {
		pkgC.subPkg = true
		if genC.action == "build" {
			pkgC.force = false
		}
	}

	if !pkgC.subPkg {
		pkgC.src, pkgC.steps = parsePkgIni(genC, pkg, pkgC)
	}

	pkgC.cfgFiles = getPkgCfgFiles(genC, pkg)

	_, pkgC.libDeps = getPkgLibDeps(genC, pkg)
	pkgC.runDeps = genC.runDeps[pkg]
	pkgC.buildDeps = genC.buildDeps[pkg]
	pkgC.allRunDeps = append(pkgC.libDeps, pkgC.runDeps...)

	sort.Slice(pkgC.allRunDeps, func(i, j int) bool {
		return pkgC.allRunDeps[i].name <= pkgC.allRunDeps[j].name
	})

	return pkgC
}

func getRootDir(args argsT, buildEnv string) string {
	var rootDir string

	if *args.rootDir == "" {
		rootDir = "/tmp/xx/" + buildEnv
	} else if strings.Contains(*args.rootDir, ":/") {
		// just clean up the path when the host is remote
		split := strings.Split(*args.rootDir, ":")
		host := split[0]
		path := split[1]
		path = filepath.Clean(path)
		rootDir = host + ":" + path
	} else {
		rootDir = filepath.Clean(*args.rootDir)
	}

	return rootDir
}

func printUsage() {
	fmt.Println(`usage: xx <action> [parameters] <xx set file or a program>

actions/parameters:

(b)uild		build packages
-f		force build of all packages
-s		build set to build for a single package (default: std)
-V		version to build for a single package

(i)nstall	install already built packages
-f		force installation of all packages
-r		root dir to install packages to
-c		system config dir
-P		only set system permissions
-C		only install configs from the system config dir

(s)ource	download and verify source files
-d		download source files

(d)iff		show diff between pkg builds
-h		time horizon in hours to search for diffs (default: 24)
-b		show diff against previous build (default: previous version)

i(n)fo		show additional information on xx system
-d		information on dependencies
-i		verify system integrity

(u)pdate	update ini files
-i		get information on latest packages available

general parameters:
-v		verbose messages`)
}

func testTools(args argsT) {

	var genC genCfgT
	genC.rootDir = "/tmp/xx/tools_test"

	err := os.MkdirAll("/tmp/xx/build", 0700)
	errExit(err, "can't create dir: /tmp/xx/build/")
	instLxcConfig(genC, pkgT{}, pkgCfgT{})

	c := `cd /home/xx/tools/ &&
	./busybox mkdir -pv /tmp/xx/tools_test/home/xx &&
	./busybox mkdir -pv /tmp/xx/tools_test/tmp &&
	./ksh -c './busybox cp -av ksh busybox /tmp/xx/tools_test/'`

	cmd := exec.Command("/bin/sh", "-c", c)
	out, err := cmd.CombinedOutput()
	errExit(err, "tools not functional:\n  "+string(out))

	// temporary change of xx permissions for a test
	err = os.Chmod("/home/xx", 0755)
	errExit(err, "can't change permissions on /home/xx/")

	c = "lxc-execute -n xx -P /tmp/ -- /ksh -c '/busybox ls /home/xx'"
	cmd = exec.Command("/bin/sh", "-c", c)
	out, err = cmd.CombinedOutput()
	outStr := string(out)
	errExit(err, "lxc not functional:\n  "+outStr+
		"\n\n  please configure lxc until you can successfully run:"+
		"\n  $ lxc-execute -n xx -P /tmp/ -- /ksh"+
		"\n\n  container path: /tmp/xx/tools_test"+
		"\n  container config: /tmp/xx/config")

	err = os.Chmod("/home/xx", 0700)
	errExit(err, "can't change permissions on /home/xx/")

	homeDirs := []string{"conf", "doc", "initramfs", "xx", "misc", "set"}
	for _, dir := range homeDirs {
		if !strings.Contains(outStr, dir) {
			msg := "/home/xx in the container is not correct"
			errExit(errors.New(""), msg)
		}
	}

	err = os.RemoveAll("/tmp/xx/tools_test")
	errExit(err, "can't remove /tmp/xx/tools_test/")

	genC.rootDir = "/tmp/xx/" + genC.buildEnv
	genC.rootDir = "/tmp/xx/" + genC.buildEnv
	instLxcConfig(genC, pkgT{}, pkgCfgT{})
}
