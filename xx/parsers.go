package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path"
	fp "path/filepath"
	"regexp"
	str "strings"
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
	fields := str.Split(line, "\t")
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

	if str.Contains(flags, "f") {
		force = true
		flags = str.Replace(flags, "f", "", 1)
	}
	if str.Contains(flags, "c") {
		cnt = true
		flags = str.Replace(flags, "c", "", 1)
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
		fields := str.Split(line, "\t")
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
	var stepsSl = make(map[string]string)
	var src srcT
	var section, step, varsVar string
	vars := make(map[string]string)

	check := map[string]bool{
		"hasSrc":            false,
		"hasUrl":            false,
		"hasSrcType":        false,
		"hasSrcDirName":     false,
		"hasVars":           false,
		"hasSet":            false,
		"has_env":           false,
		"has_prepare":       false,
		"has_configure":     false,
		"has_build":         false,
		"has_pkg_create":    false,
		"nonEmptySrcType":   false,
		"nonEmptyPkgCreate": false,
	}

	tomlFile := fp.Join(pkg.progDir, pkg.ver+".toml")

	re := getRegexes()
	f, err := os.Open(tomlFile)
	errExit(err, "can't open file: "+tomlFile)
	defer f.Close()

	input := bufio.NewScanner(f)
	var i int
	for input.Scan() {
		i++
		line := input.Text()
		line = re.wSpaces.ReplaceAllString(line, " ")
		line = str.Trim(line, " ")

		if line == "" || string(line[0]) == "#" {
			continue
		}

		// replace predefined variables
		line = replaceTomlVars(line, genC, pkg, pkgC)

		switch {
		// start of the new section
		case str.HasPrefix(line, "[ ") && str.HasSuffix(line, " ]"):
			section = str.TrimPrefix(line, "[ ")
			section = str.TrimSuffix(section, " ]")
			if str.Contains(section, " ") {
				msg := "name has a space in line %d of %s"
				errExit(fmt.Errorf(msg, i, tomlFile), "")
			}
			step, varsVar = "", ""
			switch section {
			case "src":
				check["hasSrc"] = true
			case "vars":
				check["hasVars"] = true
			case pkg.set:
				check["hasSet"] = true
			}

			if _, done := stepsSl["pkg_create"]; done {
				break
			}

		case section == "src" && str.HasPrefix(line, "url ="):
			src.url = str.TrimPrefix(line, "url = ")
			fileBase := path.Base(str.Split(src.url, " ")[0])
			src.file = fp.Join(pkg.progDir, "src", fileBase)
			step = "url"
			check["hasUrl"] = true

		case section == "src" && str.HasPrefix(line, "src_type = "):
			src.srcType = str.TrimPrefix(line, "src_type = ")
			step = "src_type"
			check["hasSrcType"] = true

		case section == "src" && str.HasPrefix(line, "src_dirname ="):
			src.dirName = str.TrimPrefix(line, "src_dirname =")
			src.dirName = str.Trim(src.dirName, " ")
			step = "src_dirname"
			check["hasSrcDirName"] = true

		case section == "src" && step == "url":
			src.url += " " + line

		case section == "vars" && str.HasPrefix(line, "var "):
			before, after, found := str.Cut(line, " = ")
			varsVar = before
			vars[varsVar] = after
			if !found {
				msg := "incorrect var name in line %d of %s"
				errExit(fmt.Errorf(msg, i, tomlFile), "")
			}

		case section == "vars":
			vars[varsVar] += " " + line

		case pkg.set == section && startStepLine(line):
			before, after, found := str.Cut(line, " =")
			step = before
			stepsSl[step] = str.Trim(after, " ")
			if !found {
				msg := "incorrect step in line %d of %s"
				errExit(fmt.Errorf(msg, i, tomlFile), "")
			}
			check["has_"+step] = true

		case pkg.set == section:
			stepsSl[step] += " " + line
		}
	}

	if src.srcType != "" {
		check["nonEmptySrcType"] = true
	}

	if stepsSl["pkg_create"] != "" || src.srcType == "files" {
		check["nonEmptyPkgCreate"] = true
	}

	// check if all's ok
	for c, val := range check {
		if !val {
			msg := "check %s failed in %s"
			errExit(fmt.Errorf(msg, c, tomlFile), "")
		}
	}

	// replace user defined vars in each step
	for step, val := range stepsSl {
		for varsVar, varVal := range vars {
			stepsSl[step] = str.Replace(val, varsVar, varVal, -1)
		}
	}

	env, err := getIniEnv(stepsSl["env"])
	errExit(err, fmt.Sprintf("incorrect env %s", tomlFile))

	steps.env = prepareEnv(env, genC, pkg, pkgC)
	steps.buildDir = fp.Join(pkgC.tmpDir, src.dirName)

	steps.prepare = stepsSl["prepare"]
	steps.configure = stepsSl["configure"]
	steps.build = stepsSl["build"]
	steps.pkg_create = stepsSl["pkg_create"]

	return src, steps
}

func getIniEnv(s string) ([]string, error) {
	var env []string
	var v string
	var isQuoted bool

	fields := str.Split(s, " ")
	for _, f := range fields {
		if f == "" {
			continue
		}

		if str.Contains(f, "=\"") || str.Contains(f, "='") {
			isQuoted = true
			v = f
			if str.HasSuffix(f, "\"") || str.HasSuffix(f, "'") {
				if !str.Contains(f, "=") {
					msg := "envvar with no '='"
					return env, errors.New(msg)
				}
				env = append(env, f)
				isQuoted = false
			}
			continue
		}

		if !isQuoted {
			if !str.Contains(f, "=") {
				return env, errors.New("envvar with no '='")
			}
			env = append(env, f)
			continue
		}

		if str.HasSuffix(f, "\"") || str.HasSuffix(f, "'") {
			v += " " + f
			if !str.Contains(v, "=") {
				return env, errors.New("envvar with no '='")
			}
			env = append(env, v)
			isQuoted = false
			v = ""
		} else {
			v += " " + f
		}
	}
	return env, nil
}

func startStepLine(line string) bool {
	stepsNames := []string{"env", "prepare", "configure",
		"build", "pkg_create"}
	for _, name := range stepsNames {
		if str.HasPrefix(line, name+" =") {
			return true
		}
	}
	return false
}

func replaceTomlVars(s string, genC genCfgT, pkg pkgT, pkgC pkgCfgT) string {
	replMap := setReplMap(genC, pkg, pkgC)

	for k, v := range replMap {
		if v != "" {
			s = str.Replace(s, k, v, -1)
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
		"<src_path>":    pkgC.src.file,
		"<tmp_dir>":     pkgC.tmpDir,
		"<build_dir>":   pkgC.steps.buildDir,
	}
}

func getPkgSpecVer(pkg pkgT) string {
	var v string

	switch pkg.prog {
	case "sqlite":
		v = str.Replace(pkg.ver, ".", "", -1) + "000"
	case "libnl":
		v = str.Replace(pkg.ver, ".", "_", -1)
	case "cdrtools":
		v = str.Split(pkg.ver, "a")[0]
	case "c-ares":
		v = str.Replace(pkg.ver, ".", "_", -1)
	case "doxygen":
		v = str.Replace(pkg.ver, ".", "_", -1)
	case "libcdio-paranoia":
		v = str.Replace(pkg.ver, "+", "-", -1)
	case "boost":
		v = str.Replace(pkg.ver, ".", "_", -1)
	case "libexif":
		v = str.Replace(pkg.ver, ".", "_", -1)
	case "vim":
		vSplit := str.Split(pkg.ver, ".")
		v = vSplit[0] + vSplit[1]
	case "w3m":
		v = str.Replace(pkg.ver, "+git", "-git", -1)
	case "unzip":
		v = str.Replace(pkg.ver, ".", "", -1)
	case "fetchmail":
		v = str.Replace(pkg.ver, ".", "-", -1)
	case "zip":
		v = str.Replace(pkg.ver, ".", "", -1)
	case "tinyxml":
		v = str.Replace(pkg.ver, ".", "_", -1)
	}

	return v
}

func getRegexes() reT {
	var re reT
	re.wSpaces = regexp.MustCompile(`\s+`)
	re.pkgName = regexp.MustCompile(`^[a-z0-9_]+/[\w-+]+$`)

	return re
}

func isStepStart(line string) bool {
	steps := []string{"url", "env", "prepare", "configure", "build",
		"pkg_create"}

	for _, step := range steps {
		if str.HasPrefix(line, step) {
			return true
		}
	}
	return false
}

func getWorldPkgs(genC genCfgT, instDir string) []pkgT {
	var worldPkgs []pkgT
	pkgVerMap := make(map[string]string)

	worldDir := fp.Join(instDir, "/var/xx")
	if !fileExists(worldDir) {
		return worldPkgs
	}

	dirs, err := walkDir(worldDir, "dirs")
	errExit(err, "can't walk world dir: "+worldDir)

	for _, dir := range dirs {
		dir := str.TrimPrefix(dir, worldDir+"/")
		fields := str.Split(dir, "/")
		if len(fields) != 3 {
			continue
		}
		name, pkgSetVerRel := fields[0]+"/"+fields[1], fields[2]
		fields = str.Split(pkgSetVerRel, "-")
		if len(fields) < 3 {
			errExit(errors.New(""), "can't parse line: "+dir)
		}

		set := fields[0]
		ver := str.Join(fields[1:len(fields)-1], "-")
		nameSet := name + "\t" + set

		// assuming that latest entries are installed prog versions
		pkgVerMap[nameSet] = ver
	}

	for nameSet, ver := range pkgVerMap {
		fields := str.Split(nameSet, "\t")
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
		fields := str.Split(input.Text(), "\t")
		name := fields[1]
		pkgSet := fields[2]
		dep := getPkg(genC, name, pkgSet, "latest")
		deps = append(deps, dep)
	}

	return deps
}

// get a list of all container names in /cnt/rootfs dir
func getCntList(cntDir string) []string {
	var cntList []string
	if !fileExists(cntDir) {
		return cntList
	}
	cntDirs, err := os.ReadDir(cntDir)
	errExit(err, "can't read cnt dir: "+cntDir)

	for _, cntDirEntry := range cntDirs {
		cntDirName := cntDirEntry.Name()
		if !cntDirEntry.IsDir() {
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
	l := str.Split(line, "\t")

	if len(l) != 2 {
		errExit(errors.New(""), "incorrect permissions: "+line)
	}

	var slashTrail string
	if line[len(line)-1] == '/' {
		slashTrail = "/"
	}

	if str.Contains(l[0], ":") {
		c := ""
		permPath := l[1]
		split := str.Split(permPath, ":")
		if len(split) > 1 && str.HasPrefix(permPath, "c:") {
			c = "c:"
			permPath = split[1]
		}
		permPath = c + fp.Join(rootDir, permPath) + slashTrail
		owners[permPath] = l[0]
	} else if strDigitsOnly(l[0]) {
		split := str.Split(l[1], ":")
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
			rootFile := str.TrimPrefix(file, pkg.cfgDir)
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
			rootFile := str.TrimPrefix(file, pkgSysCfgDir)
			if file == rootFile {
				errExit(errors.New(""),
					"file can't be copied from root dir")
			}
			cfgFiles[rootFile] = file
		}
	}

	return cfgFiles
}
