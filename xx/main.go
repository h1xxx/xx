package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"

	fp "path/filepath"
	str "strings"
)

// runT stores global variables for each run of the command
//
// rootDir	root system dir:		/, /mnt/xx, /tmp/xx/media
// sysCfgDir	dir with system config:		/home/xx/conf/<machine>
// buildEnv	name of the build environment:	base, init_musl, net, media
// baseDir	dir with installed base pkgs:	/tmp/xx/musl_base, /tmp/xx/base
// baseFile	file with a list of base pkgs:	/home/xx/set/base.xx
// baseEnv	if build environment is base:	false, true
// muslEnv	use musl build environment:	false, true
// isInit	bootstrapping the build env:	false, true
// actionTarget	package name or pkg env file:	sys/lvm2, base.xx, media.xx
// action	action to perform:		build, install, update, diff
// date		current calendar date:		2002-05-13
//
// forceAll	force action on all packages:	false, true
// fixedSet	fixed pkg set for all packages:	[empty], std, musl
// fixedVer	fixed pkg ver for all packages:	[empty], latest, 2.35
//
// toInstPerms	only install sys permissions:	false, true
// toInstSysCfg	only install system config:	false, true
//
// diffBuild	show diff to previous build:	false, true
// diffHours	show diff within X last hours:	24, 30, 48
//
// infoDeps	show info on dependencies	false, true
// infoInteg	check system integrity		false, true
type runT struct {
	rootDir   string
	sysCfgDir string
	buildEnv  string
	baseDir   string
	baseFile  string
	baseEnv   bool
	muslEnv   bool
	isInit    bool

	actionTarget      string
	action            string
	targetIsSinglePkg bool

	date string

	forceAll bool
	fixedSet string
	fixedVer string

	toInstPerms  bool
	toInstSysCfg bool

	diffBuild bool
	diffHours int64

	infoDeps  bool
	infoInteg bool

	pkgs      []pkgT
	pkgCfgs   []pkgCfgT
	world     map[string]worldT
	buildDeps map[pkgT][]pkgT
	runDeps   map[pkgT][]pkgT
}

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

// pkgT stores common variables to uniquely identify a package
//
// name		package name (format <category/program>):	sys/lvm2
// categ	program category: 				sys
// prog		program name:					lvm2
// set		package set:					std, init_cross
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
// patchDir	dir with patches:		/home/xx/sys/lvm2/patch/2.35
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
	patchDir   string
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
// muslBuild	program is compiled in musl libc environment
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
	muslBuild  bool
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
// srcType	type of source code:	tar, git, files, go-mod, alpine, cmd
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
// subPkgs	sub packages to create with files moved from the main pkg
type stepsT struct {
	env      []string
	buildDir string

	prepare    string
	configure  string
	build      string
	pkg_create string

	subPkgs []subPkgT
}

type subPkgT struct {
	suffix string
	files  []string
}

// reT contains regexes for various checks and subsitutions
//
// wSpaces		matches multiple whitespaces
// pkgName		matches a correct package name
type reT struct {
	wSpaces       *regexp.Regexp
	pkgName       *regexp.Regexp
	noNoSharedLib *regexp.Regexp
	noNoStaticLib *regexp.Regexp
	staticBin     *regexp.Regexp
	glibcBin      *regexp.Regexp
}

func main() {
	r := parseArgs()
	r.getRunVars()
	r.getPkgList()
	r.getWorld(r.pkgCfgs)

	r.printRunParams()

	if r.fixedSet != "xx_tools_cross" {
		testTools()
	}

	switch {
	case r.action == "build":
		r.actionBuild()

	case r.action == "install":
		r.actionInst()

	case r.action == "diff":
		r.actionDiff()

	case r.action == "update":
		r.actionUpdate()

	case r.action == "source":
		r.actionSource()

	case r.action == "check":
		r.actionCheck()

	case r.action == "info":
		r.actionInfo()
	}
}

func (r *runT) getPkgList() {
	// process a pkg env file
	if !r.targetIsSinglePkg {
		r.pkgs, r.pkgCfgs = r.parseBuildEnvFile(r.actionTarget)
	}

	// process a single package
	// todo: add deps
	if r.targetIsSinglePkg {
		pkg := r.getPkg(r.actionTarget, "std", "latest")
		pkgC := r.getPkgCfg(pkg, "")

		r.pkgs = append(r.pkgs, pkg)
		r.pkgCfgs = append(r.pkgCfgs, pkgC)
	}
}

func (r *runT) getPkg(name, pkgSet, ver string) pkgT {
	var pkg pkgT
	fields := str.Split(name, "/")

	pkg.name = name
	pkg.categ = fields[0]
	pkg.prog = fields[1]

	pkg.set = pkgSet
	if r.fixedSet != "" && r.fixedSet != "std" {
		pkg.set = r.fixedSet
	}

	pkg.progDir = fp.Join("/home/xx/prog", pkg.name)
	pkg.ver = getVer(pkg, ver)
	if r.fixedVer != "" && r.fixedVer != "latest" {
		pkg.ver = r.fixedVer
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
	pkg.patchDir = fp.Join(pkg.progDir, "patch", pkg.ver)
	pkg.pkgDir = fp.Join(pkg.progDir, "pkg", pkg.setVerRel)
	pkg.newPkgDir = fp.Join(pkg.progDir, "pkg", pkg.setVerNewRel)
	pkg.prevPkgDir = fp.Join(pkg.progDir, "pkg", pkg.setVerPrevRel)
	pkg.cfgDir = findCfgDir(fp.Join(pkg.progDir, "cfg"), pkg)

	return pkg
}

// searches for the best directory in program configs dir
func findCfgDir(searchDir string, pkg pkgT) string {
	var res string
	cfgDir := fp.Join(searchDir, pkg.set+"-"+pkg.ver)
	cfgLatestDir := fp.Join(searchDir, pkg.set+"-latest")
	cfgAllSetsDir := fp.Join(searchDir, "all-"+pkg.ver)
	cfgAllSetsLatestDir := fp.Join(searchDir, "all-latest")

	switch {
	case fileExists(cfgDir):
		res = cfgDir
	case fileExists(cfgLatestDir):
		res = cfgLatestDir
	case fileExists(cfgAllSetsDir):
		res = cfgAllSetsDir
	default:
		res = cfgAllSetsLatestDir
	}

	return res
}

func (r *runT) getPkgCfg(pkg pkgT, flags string) pkgCfgT {
	var pkgC pkgCfgT
	pkgC.force, pkgC.cnt = parsePkgFlags(flags, pkg.name)

	if r.forceAll {
		pkgC.force = true
	}

	if pkgC.cnt {
		pkgC.cntPkg = pkg
		pkgC.cntProg = pkg.prog
		pkgC.instDir = fp.Join(r.rootDir, "/cnt/rootfs/",
			pkgC.cntProg)
	} else {
		pkgC.instDir = r.rootDir
	}

	buildPath := "/tmp/xx/build/" + pkg.prog + "-" + pkg.ver + "_build-"
	buildID := getLastRel("/tmp/xx/build/",
		pkg.prog+"-"+pkg.ver+"_build-")
	if fileExists(buildPath + "00") {
		buildID += 1
	}
	pkgC.tmpDir = buildPath + fmt.Sprintf("%0.2x", buildID)
	pkgC.tmpLogDir = pkgC.tmpDir + "/log/"

	if str.HasSuffix(pkg.set, "_cross") {
		pkgC.crossBuild = true
	}

	if str.HasPrefix(pkg.set, "musl") {
		pkgC.muslBuild = true
	}

	pkgC.src, pkgC.steps, pkgC.subPkg = r.parsePkgIni(pkg, pkgC)
	pkgC.cfgFiles = r.getPkgCfgFiles(pkg)

	_, pkgC.libDeps = r.getPkgLibDeps(pkg)
	pkgC.runDeps = r.runDeps[pkg]
	pkgC.buildDeps = r.buildDeps[pkg]
	pkgC.allRunDeps = append(pkgC.libDeps, pkgC.runDeps...)

	sort.Slice(pkgC.allRunDeps, func(i, j int) bool {
		return pkgC.allRunDeps[i].name <= pkgC.allRunDeps[j].name
	})

	return pkgC
}

func testTools() {
	var r runT
	r.rootDir = "/tmp/xx/tools_test"

	err := os.MkdirAll("/tmp/xx/build", 0700)
	errExit(err, "can't create dir: /tmp/xx/build/")
	r.instLxcConfig(pkgT{}, pkgCfgT{})

	c := `cd /home/xx/bin/ &&
	./busybox mkdir -pv /tmp/xx/tools_test/home/xx &&
	./busybox mkdir -pv /tmp/xx/tools_test/tmp &&
	./bash -c './busybox cp -av bash busybox /tmp/xx/tools_test/'`

	cmd := exec.Command("/bin/sh", "-c", c)
	out, err := cmd.CombinedOutput()
	errExit(err, "tools not functional:\n  "+string(out))

	// temporary change of xx permissions for a test
	err = os.Chmod("/home/xx", 0755)
	errExit(err, "can't change permissions on /home/xx/")

	c = "lxc-execute -n xx -P /tmp/ -- /bash -c '/busybox ls /home/xx'"
	cmd = exec.Command("/bin/sh", "-c", c)
	out, err = cmd.CombinedOutput()
	outStr := string(out)
	errExit(err, "lxc not functional:\n  "+outStr+
		"\n\n  please configure lxc until you can successfully run:"+
		"\n  $ lxc-execute -n xx -P /tmp/ -- /bash"+
		"\n\n  container path: /tmp/xx/tools_test"+
		"\n  container config: /tmp/xx/config")

	err = os.Chmod("/home/xx", 0700)
	errExit(err, "can't change permissions on /home/xx/")

	homeDirs := []string{"cfg", "doc", "initramfs", "xx", "misc", "set"}
	for _, dir := range homeDirs {
		if !str.Contains(outStr, dir) {
			msg := "/home/xx in the container is mounted correctly"
			errExit(errors.New(""), msg)
		}
	}

	err = os.RemoveAll("/tmp/xx/tools_test")
	errExit(err, "can't remove /tmp/xx/tools_test/")

	r.rootDir = "/tmp/xx/" + r.buildEnv
	r.rootDir = "/tmp/xx/" + r.buildEnv
	r.instLxcConfig(pkgT{}, pkgCfgT{})
}
