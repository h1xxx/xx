package main

import (
	"bufio"
	"errors"
	"os"
	"path"
	fp "path/filepath"
	"regexp"
	"strings"

	toml "github.com/pelletier/go-toml"
)

func parsePkgEnvFile(xxFile string, genC genCfgT) ([]pkgT, []pkgCfgT) {
	var pkgs []pkgT
	var pkgCfgs []pkgCfgT

	re := getRegexes()

	f, err := os.Open(xxFile)
	errExit(err, "can't open file: "+xxFile)
	defer f.Close()

	input := bufio.NewScanner(f)
	for input.Scan() {
		line := input.Text()
		if line == "" || string(line[0]) == "#" {
			continue
		}

		pkg, pkgC := parseSetLine(line, genC, re)

		pkgs = append(pkgs, pkg)
		pkgCfgs = append(pkgCfgs, pkgC)
	}
	return pkgs, pkgCfgs
}

func parseSetLine(line string, genC genCfgT, re reT) (pkgT, pkgCfgT) {
	line = re.wSpaces.ReplaceAllString(line, "\t")
	fields := strings.Split(line, "\t")
	len := len(fields)
	var flags string
	err := errors.New("")

	switch {
	case len == 4:
		flags = fields[3]
	case len < 3:
		errExit(err, "too few fields in line:\n  "+line)
	case len > 4:
		errExit(err, "too many fields in line:\n  "+line)
	}

	name := fields[0]
	pkgSet := fields[1]
	ver := fields[2]

	// todo: move this to some central pkg checking place
	correctPkgName := re.pkgName.MatchString(name)
	if !correctPkgName {
		errExit(err, "incorrect pkg name:\n  "+name)
	}

	pkg := getPkg(genC, name, pkgSet, ver)
	pkgC := getPkgCfg(genC, pkg, flags)

	// todo: implement check pkg and pkgC functions
	// e.g. (*args.ver != "latest" || *args.set != "std")
	// e.g. non-empty fields, match predefined regexes etc.

	return pkg, pkgC
}

func parsePkgFlags(flags, pkgName string) (bool, bool) {
	var force, cnt bool

	if strings.Contains(flags, "f") {
		force = true
		flags = strings.Replace(flags, "f", "", 1)
	}
	if strings.Contains(flags, "c") {
		cnt = true
		flags = strings.Replace(flags, "c", "", 1)
	}
	if flags != "" {
		errExit(errors.New(""), "unknown flags in:\n  "+pkgName)
	}

	return force, cnt
}

func parseCntConf(cntConf string) (map[string]string, map[string]string) {
	binCnt := make(map[string]string)
	cntIP := make(map[string]string)
	re := getRegexes()

	f, err := os.Open(cntConf)
	errExit(err, "can't open file: "+cntConf)
	defer f.Close()

	input := bufio.NewScanner(f)
	for input.Scan() {
		line := input.Text()

		if line == "" || string(line[0]) == "#" {
			continue
		}

		line = re.wSpaces.ReplaceAllString(line, "\t")
		fields := strings.Split(line, "\t")
		len := len(fields)

		switch {
		case len < 3:
			errExit(errors.New(""),
				"too few fields in line:\n  "+line)
		case len > 4:
			errExit(errors.New(""),
				"too many fields in line:\n  "+line)
		}

		bin := fields[0]
		cntName := fields[1]
		ip := fields[2]

		binCnt[bin] = cntName
		cntIP[cntName] = ip
	}

	return binCnt, cntIP
}

func parsePkgToml(genC genCfgT, pkg pkgT, pkgC pkgCfgT) (srcT, stepsT) {
	var steps stepsT
	var src srcT

	tomlFile := pkg.progDir + "/" + pkg.ver + ".toml"
	tomlStr := readToml(tomlFile)

	// replace already known variables in toml and parse it
	tomlStr = replaceTomlVars(tomlStr, genC, pkg, pkgC)
	conf, err := toml.Load(tomlStr)
	errExit(err, "can't parse toml file")
	checkConf(pkg, conf)
	confSrc := conf.Get("src").(*toml.Tree)

	src.url = confSrc.Get("url").(string)
	srcFileBase := path.Base(strings.Split(src.url, " ")[0])
	src.srcFile = fp.Join(pkg.progDir, "src", srcFileBase)
	src.srcDirName = confSrc.Get("src_dirname").(string)
	src.srcType = confSrc.Get("src_type").(string)

	steps.buildDir = fp.Join(pkgC.tmpDir, src.srcDirName)
	pkgC.src = src
	pkgC.steps = steps

	// replace also new src variables in toml string and reparse it
	// todo: check if this algorithm needs to be sped up
	tomlStr = replaceTomlVars(tomlStr, genC, pkg, pkgC)
	conf, err = toml.Load(tomlStr)
	errExit(err, "can't parse toml file")
	checkConf(pkg, conf)
	confStep := conf.Get(pkg.set).(*toml.Tree)

	steps.prepare = confStep.Get("prepare").(string)
	steps.configure = confStep.Get("configure").(string)
	steps.build = confStep.Get("build").(string)
	steps.pkg_create = confStep.Get("pkg_create").(string)

	// todo: place default configs in a file, read defaults into genC
	var env []string
	envIface := confStep.Get("env").([]interface{})
	for i := range envIface {
		env = append(env, envIface[i].(string))
	}
	steps.env = prepareEnv(env, genC, pkgC)

	return src, steps
}

func replaceTomlVars(s string, genC genCfgT, pkg pkgT, pkgC pkgCfgT) string {
	replMap := setReplMap(genC, pkg, pkgC)

	for k, v := range replMap {
		if v != "" {
			s = strings.Replace(s, k, v, -1)
		}
	}

	return s
}

func setReplMap(genC genCfgT, pkg pkgT, pkgC pkgCfgT) map[string]string {
	return map[string]string{
		"<root_dir>":    genC.rootDir,
		"<prog>":        pkg.prog,
		"<ver>":         pkg.ver,
		"<ver_short>":   pkg.verShort,
		"<pkg_rel>":     pkg.newRel,
		"<set_ver_rel>": pkg.setVerNewRel,
		"<pkg_dir>":     pkg.newPkgDir,
		"<prog_dir>":    pkg.progDir,
		"<src_dir>":     fp.Join(pkg.progDir, "src"),
		"<ver_pkgspec>": getPkgSpecVer(pkg),
		"<src_path>":    pkgC.src.srcFile,
		"<tmp_dir>":     pkgC.tmpDir,
		"<build_dir>":   pkgC.steps.buildDir,
	}
}

func getPkgSpecVer(pkg pkgT) string {
	var v string

	switch pkg.prog {
	case "sqlite":
		v = strings.Replace(pkg.ver, ".", "", -1) + "000"
	case "libnl":
		v = strings.Replace(pkg.ver, ".", "_", -1)
	case "cdrtools":
		v = strings.Split(pkg.ver, "a")[0]
	case "c-ares":
		v = strings.Replace(pkg.ver, ".", "_", -1)
	case "doxygen":
		v = strings.Replace(pkg.ver, ".", "_", -1)
	case "libcdio-paranoia":
		v = strings.Replace(pkg.ver, "+", "-", -1)
	case "boost":
		v = strings.Replace(pkg.ver, ".", "_", -1)
	case "libexif":
		v = strings.Replace(pkg.ver, ".", "_", -1)
	case "vim":
		vSplit := strings.Split(pkg.ver, ".")
		v = vSplit[0] + vSplit[1]
	case "w3m":
		v = strings.Replace(pkg.ver, "+git", "-git", -1)
	case "unzip":
		v = strings.Replace(pkg.ver, ".", "", -1)
	case "fetchmail":
		v = strings.Replace(pkg.ver, ".", "-", -1)
	case "zip":
		v = strings.Replace(pkg.ver, ".", "", -1)
	case "tinyxml":
		v = strings.Replace(pkg.ver, ".", "_", -1)
	}

	return v
}

func getRegexes() reT {
	var re reT
	re.wSpaces = regexp.MustCompile(`\s+`)
	re.pkgName = regexp.MustCompile(`^[a-z0-9_]+/[\w-+]+$`)

	return re
}

// reads xs toml file and escapes newlines to be compliant with toml format
func readToml(file string) string {
	var tomlStr string
	var isStep bool

	fd, err := os.Open(file)
	errExit(err, "can't open toml file")
	defer fd.Close()

	input := bufio.NewScanner(fd)
	for input.Scan() {
		line := input.Text()
		if isStepStart(line) {
			isStep = true
		}
		if isStepEnd(line) {
			isStep = false
		}
		if isStep {
			tomlStr += line + " \\\n"
		} else {
			tomlStr += line + "\n"
		}
	}
	return tomlStr
}

func isStepStart(line string) bool {
	steps := []string{"url", "env", "prepare", "configure", "build",
		"pkg_create"}

	for _, step := range steps {
		if strings.HasPrefix(line, step) {
			return true
		}
	}
	return false
}

func isStepEnd(line string) bool {
	line = strings.TrimSpace(line)
	if strings.HasSuffix(line, "\"") && !strings.HasSuffix(line, "\\\"") {
		return true
	}

	// this is for env variable, but the whole funcion doesn't cover all
	// cases, todo: make it more robust
	if strings.HasSuffix(line, "]") && !strings.HasSuffix(line, "\\]") {
		return true
	}

	return false
}

func getWorldPkgs(genC genCfgT, instDir string) []pkgT {
	var worldPkgs []pkgT
	pkgVerMap := make(map[string]string)

	worldDir := fp.Join(instDir, "/var/lib/xx")
	if !fileExists(worldDir) {
		return worldPkgs
	}

	dirs, err := walkDir(worldDir, "dirs")
	errExit(err, "can't walk world dir: "+worldDir)

	for _, dir := range dirs {
		dir := strings.TrimPrefix(dir, worldDir+"/")
		fields := strings.Split(dir, "/")
		if len(fields) != 3 {
			continue
		}
		name, pkgSetVerRel := fields[0]+"/"+fields[1], fields[2]
		fields = strings.Split(pkgSetVerRel, "-")
		if len(fields) < 3 {
			errExit(errors.New(""), "can't parse line: "+dir)
		}

		set := fields[0]
		ver := strings.Join(fields[1:len(fields)-1], "-")
		nameSet := name + "\t" + set

		// assuming that latest entries are installed prog versions
		pkgVerMap[nameSet] = ver
	}

	for nameSet, ver := range pkgVerMap {
		fields := strings.Split(nameSet, "\t")
		name := fields[0]
		set := fields[1]
		pkg := getPkg(genC, name, set, ver)

		worldPkgs = append(worldPkgs, pkg)
	}

	return worldPkgs
}

//
func parseSharedLibsFile(genC genCfgT, sharedLibsFile string) []pkgT {
	var deps []pkgT

	fd, err := os.Open(sharedLibsFile)
	errExit(err, "can't open shared_libs file "+sharedLibsFile)
	defer fd.Close()
	input := bufio.NewScanner(fd)

	for input.Scan() {
		fields := strings.Split(input.Text(), "\t")
		name := fields[1]
		pkgSet := fields[2]
		dep := getPkg(genC, name, pkgSet, "latest")
		deps = append(deps, dep)
	}

	return deps
}

// get a list of all container names in root's /usr/cnt dir
func getCntList(cntDir string) []string {
	var cntList []string
	if !fileExists(cntDir) {
		return cntList
	}
	cntDirs, err := os.ReadDir(cntDir)
	errExit(err, "can't read cnt dir: "+cntDir)

	for _, cntDirEntry := range cntDirs {
		cntDirName := cntDirEntry.Name()
		if !cntDirEntry.IsDir() || cntDirName == "bin" {
			continue
		}
		cntList = append(cntList, cntDirName)
	}

	return cntList
}

func readPermsFile(file, rootDir string, perms, owners map[string]string) {
	f, err := os.Open(file)
	errExit(err, "can't open file: "+file)
	defer f.Close()

	re := getRegexes()

	input := bufio.NewScanner(f)
	for input.Scan() {
		line := input.Text()
		parsePermLine(line, rootDir, perms, owners, re)
	}
}

func parsePermLine(line, rootDir string, perms, owners map[string]string,
	re reT) {

	if line == "" || string(line[0]) == "#" {
		return
	}
	line = re.wSpaces.ReplaceAllString(line, "\t")
	l := strings.Split(line, "\t")

	if len(l) != 2 {
		errExit(errors.New(""), "incorrect permissions: "+line)
	}

	var slashTrail string
	if line[len(line)-1] == '/' {
		slashTrail = "/"
	}

	if strings.Contains(l[0], ":") {
		c := ""
		permPath := l[1]
		split := strings.Split(permPath, ":")
		if len(split) > 1 && strings.HasPrefix(permPath, "c:") {
			c = "c:"
			permPath = split[1]
		}
		permPath = c + fp.Join(rootDir, permPath) + slashTrail
		owners[permPath] = l[0]
	} else if strDigitsOnly(l[0]) {
		split := strings.Split(l[1], ":")
		if len(split) != 2 {
			errExit(errors.New(""),
				"incorrect permissions: "+line)
		}
		pathType := split[0]
		permPath := split[1]
		perms[pathType+":"+fp.Join(rootDir, permPath)+slashTrail] = l[0]
	} else {
		errExit(errors.New(""), "incorrect permissions: "+line)
	}
}

// returns a map of config files to be installed with a pkg
// file in root dir -> location of the file
func getPkgCfgFiles(genC genCfgT, pkg pkgT) map[string]string {
	cfgFiles := make(map[string]string)

	// get files from pkg cfg dir
	if fileExists(pkg.cfgDir) {
		files, err := walkDir(pkg.cfgDir, "files")
		errExit(err, "can't walk pkg cfg dir: "+pkg.cfgDir)
		for _, file := range files {
			rootFile := strings.TrimPrefix(file, pkg.cfgDir)
			if file == rootFile {
				errExit(errors.New(""),
					"file can't be copied from root dir")
			}
			cfgFiles[rootFile] = file
		}
	}

	// get files from system cfg dir for the pkg
	pkgSysCfgDir := fp.Join(genC.sysCfgDir, pkg.prog, pkg.set+"-"+pkg.ver)
	if !fileExists(pkgSysCfgDir) {
		pkgSysCfgDir = fp.Join(genC.sysCfgDir, pkg.prog,
			pkg.set+"-latest")
	}

	if fileExists(pkgSysCfgDir) {
		files, err := walkDir(pkgSysCfgDir, "files")
		errExit(err, "can't walk sys cfg dir: "+pkgSysCfgDir)
		for _, file := range files {
			rootFile := strings.TrimPrefix(file, pkgSysCfgDir)
			if file == rootFile {
				errExit(errors.New(""),
					"file can't be copied from root dir")
			}
			cfgFiles[rootFile] = file
		}
	}

	return cfgFiles
}
