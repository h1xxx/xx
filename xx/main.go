package main

import (
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
// isSepSys	build for non-xx system:	false, true
// actionTarget	package name or pkg env file:	sys/lvm2, base.xx, media.xx
// action	action to perform:		build, install, update, diff
// date		current calendar date:		2002-05-13
//
// baseOk       base dir is ready for use       false, true
// baseLinked   base dir is linked to root dir  false, true
//
// forceAll	force action on all packages:	false, true
// fixedSet	fixed pkg set for all packages:	[empty], std, musl
// fixedVer	fixed pkg ver for all packages:	[empty], latest, 2.35
//
// toInstPerms	only install sys permissions:	false, true
// toInstSysCfg	only install system config:	false, true
// installCnt	install pkgs have containers:	false, true
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
	isSepSys  bool

	baseOk     bool
	baseLinked bool

	actionTarget      string
	action            string
	targetIsSinglePkg bool

	date string
	re   reT

	forceAll bool
	fixedSet string
	fixedVer string

	toInstPerms  bool
	toInstSysCfg bool
	installCnt   bool

	diffBuild bool
	diffHours int64

	infoDeps  bool
	infoInteg bool

	pkgs      []pkgT
	pkgCfgs   []pkgCfgT
	cnts      []string
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
	gitVer        *regexp.Regexp
	noNoSharedLib *regexp.Regexp
	noNoStaticLib *regexp.Regexp
	staticBin     *regexp.Regexp
	glibcBin      *regexp.Regexp
}

var ERR error

func main() {
	r := parseArgs()
	r.getRunVars()
	r.getPkgList()
	r.getWorld(r.pkgCfgs)
	r.detectSeparateSys()

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
		p := r.getPkg(r.actionTarget, "std", "latest")
		pc := r.getPkgCfg(p, "")

		r.pkgs = append(r.pkgs, p)
		r.pkgCfgs = append(r.pkgCfgs, pc)
	}
}

func (r *runT) getPkg(name, pkgSet, ver string) pkgT {
	var p pkgT
	fields := str.Split(name, "/")

	p.name = name
	p.categ = fields[0]
	p.prog = fields[1]

	p.set = pkgSet
	if r.fixedSet != "" && r.fixedSet != "std" {
		p.set = r.fixedSet
	}

	p.progDir = fp.Join("/home/xx/prog", p.name)
	p.ver = getVer(p, ver)
	if r.fixedVer != "" && r.fixedVer != "latest" {
		p.ver = r.fixedVer
	}
	p.verShort = getVerShort(p.ver, r.re.gitVer)

	p.rel, p.prevRel, p.newRel = getPkgRels(p)
	p = getPkgSetVers(p)
	p = getPkgDirs(p)

	return p
}

func getPkgSetVers(p pkgT) pkgT {
	setVer := p.set + "-" + p.ver
	p.setVerRel = setVer + "-" + p.rel
	p.setVerPrevRel = setVer + "-" + p.prevRel
	p.setVerNewRel = setVer + "-" + p.newRel

	return p
}

func getPkgDirs(p pkgT) pkgT {
	p.patchDir = fp.Join(p.progDir, "patch", p.ver)
	p.pkgDir = fp.Join(p.progDir, "pkg", p.setVerRel)
	p.newPkgDir = fp.Join(p.progDir, "pkg", p.setVerNewRel)
	p.prevPkgDir = fp.Join(p.progDir, "pkg", p.setVerPrevRel)
	p.cfgDir = findCfgDir(fp.Join(p.progDir, "cfg"), p)

	return p
}

// searches for the best directory in program configs dir
func findCfgDir(searchDir string, p pkgT) string {
	var res string

	cfgDir := fp.Join(searchDir, p.set+"-"+p.ver)
	cfgLatestDir := fp.Join(searchDir, p.set+"-latest")
	cfgAllSetsDir := fp.Join(searchDir, "all-"+p.ver)
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

func (r *runT) getPkgCfg(p pkgT, flags string) pkgCfgT {
	var pc pkgCfgT
	pc.force, pc.cnt = parsePkgFlags(flags, p.name)

	if r.forceAll {
		pc.force = true
	}

	if pc.cnt {
		pc.cntPkg = p
		pc.cntProg = p.prog
		pc.instDir = fp.Join(r.rootDir, "/cnt/rootfs/", pc.cntProg)
	} else {
		pc.instDir = r.rootDir
	}

	buildPath := "/tmp/xx/build/" + p.prog + "-" + p.ver + "_build-"
	buildID := getLastRel("/tmp/xx/build/", p.prog+"-"+p.ver+"_build-")
	if fileExists(buildPath + "00") {
		buildID += 1
	}
	pc.tmpDir = buildPath + fmt.Sprintf("%0.2x", buildID)
	pc.tmpLogDir = pc.tmpDir + "/log/"

	if str.HasSuffix(p.set, "_cross") {
		pc.crossBuild = true
	}

	if str.HasPrefix(p.set, "musl") {
		pc.muslBuild = true
	}

	pc.src, pc.steps, pc.subPkg = r.parsePkgIni(p, pc)
	pc.cfgFiles = r.getPkgCfgFiles(p)

	_, pc.libDeps = r.getPkgLibDeps(p)
	pc.runDeps = r.runDeps[p]
	pc.buildDeps = r.buildDeps[p]
	pc.allRunDeps = append(pc.libDeps, pc.runDeps...)

	sort.Slice(pc.allRunDeps, func(i, j int) bool {
		return pc.allRunDeps[i].name <= pc.allRunDeps[j].name
	})

	return pc
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
			errExit(ERR, msg)
		}
	}

	err = os.RemoveAll("/tmp/xx/tools_test")
	errExit(err, "can't remove /tmp/xx/tools_test/")

	r.rootDir = "/tmp/xx/" + r.buildEnv
	r.rootDir = "/tmp/xx/" + r.buildEnv
	r.instLxcConfig(pkgT{}, pkgCfgT{})
}
