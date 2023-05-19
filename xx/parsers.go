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

func (r *runT) parseBuildEnvFile(xxFile string) ([]pkgT, []pkgCfgT) {
	var pkgs []pkgT
	var pkgCfgs []pkgCfgT
	var cnts = make(map[string]bool)

	f, err := os.Open(xxFile)
	errExit(err, "can't open file:", xxFile)
	defer f.Close()

	input := bufio.NewScanner(f)
	for input.Scan() {
		line := input.Text()
		if line == "" || string(line[0]) == "#" {
			continue
		}

		p, pc := r.parseSetLine(line, r.re)
		if pc.cnt {
			r.installCnt = true
			cnts[pc.cntProg] = true
		}

		pkgs = append(pkgs, p)
		pkgCfgs = append(pkgCfgs, pc)
	}

	for cnt, _ := range cnts {
		r.cnts = append(r.cnts, cnt)
	}

	return pkgs, pkgCfgs
}

func (r *runT) parseSetLine(line string, re reT) (pkgT, pkgCfgT) {
	line = re.wSpaces.ReplaceAllString(line, "\t")
	fields := str.Split(line, "\t")
	len := len(fields)
	var flags string

	switch {
	case len == 4:
		flags = fields[3]
	case len < 3:
		errExit(ERR, "too few fields in line:", line)
	case len > 4:
		errExit(ERR, "too many fields in line:", line)
	}

	name := fields[0]
	pkgSet := fields[1]
	ver := fields[2]

	// todo: move this to some central pkg checking place
	correctPkgName := re.pkgName.MatchString(name)
	if !correctPkgName {
		errExit(ERR, "incorrect pkg name:", name)
	}

	p := r.getPkg(name, pkgSet, ver)
	pc := r.getPkgCfg(p, flags)

	// todo: implement check pkg and pkgC functions
	// e.g. (*args.ver != "latest" || *args.set != "std")
	// e.g. non-empty fields, match predefined regexes etc.

	return p, pc
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
		errExit(ERR, "unknown flags in:", pkgName)
	}

	return force, cnt
}

// returns src, steps and isSubPkg
func (r *runT) parsePkgIni(p pkgT, pc pkgCfgT) (srcT, stepsT, bool) {
	var steps stepsT
	var stepsMap = make(map[string]string)
	var src srcT
	var section, step, varsVar string
	var subPkgList []string
	vars := make(map[string]string)
	set := p.set

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

	iniFile := fp.Join(p.progDir, p.ver+".ini")
	f, err := os.Open(iniFile)
	errExit(err, "can't open file: "+iniFile)
	defer f.Close()

	input := bufio.NewScanner(f)
	var i int
	for input.Scan() {
		i++
		line := input.Text()
		line = r.re.wSpaces.ReplaceAllString(line, " ")
		line = str.Trim(line, " ")

		if line == "" || string(line[0]) == "#" {
			continue
		}

		// replace predefined variables
		line = r.replaceIniVars(line, p, pc, src, steps)

		switch {
		// start of the new section
		case str.HasPrefix(line, "[ ") && str.HasSuffix(line, " ]"):
			section = str.TrimPrefix(line, "[ ")
			section = str.TrimSuffix(section, " ]")
			if str.Contains(section, " ") {
				msg := "name has a space in line %d of %s"
				errExit(fmt.Errorf(msg, i, iniFile))
			}
			step, varsVar = "", ""
			switch section {
			case "src":
				check["hasSrc"] = true
			case "vars":
				check["hasVars"] = true
			case set:
				check["hasSet"] = true
			}

			// don't parse if next section starts and we have
			// everything we need from the requested set
			if _, done := stepsMap["pkg_create"]; done {
				break
			}

		case section == "src" && str.HasPrefix(line, "url ="):
			src.url = str.TrimPrefix(line, "url = ")
			fileBase := path.Base(str.Split(src.url, " ")[0])
			src.file = fp.Join(p.progDir, "src", fileBase)
			step = "url"
			if src.url != "" || src.srcType == "files" {
				check["hasUrl"] = true
			}

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
			varsVar = str.TrimPrefix(before, "var ")
			if varsVar == "pkgset_"+set {
				set = after
				continue
			}
			vars[varsVar] = after
			if !found {
				msg := "incorrect var name in line %d of %s"
				errExit(fmt.Errorf(msg, i, iniFile))
			}

		case section == "vars":
			vars[varsVar] += " " + line

		case set == section && startStepLine(line):
			before, after, found := str.Cut(line, " =")
			val := str.Trim(after, " ")
			step = before
			stepsMap[step] = val
			if !found {
				msg := "incorrect step in line %d of %s"
				errExit(fmt.Errorf(msg, i, iniFile))
			}
			check["has_"+step] = true
			if str.HasPrefix(step, "subpkg_") {
				subPkgList = append(subPkgList, step)
			}

		case set == section:
			stepsMap[step] += " " + line

		// if step is a subpkg defintion then stop parsing, no need
		case lineIsSubPkgDef(line, set, section):
			return src, steps, true

		}
	}

	if !check["hasSet"] {
		errExit(ERR, "config set", p.set, "missing in", iniFile)
	}

	if src.srcType != "" {
		check["nonEmptySrcType"] = true
	}

	if stepsMap["pkg_create"] != "" || src.srcType == "files" {
		check["nonEmptyPkgCreate"] = true
	}

	// check if all's ok
	for c, val := range check {
		if !val {
			errExit(ERR, "check", c, "failed in", iniFile)
		}
	}

	// replace user defined vars in each step
	for step, _ := range stepsMap {
		for varsVar, varVal := range vars {
			varsVar = "<" + varsVar + ">"
			stepsMap[step] = str.Replace(stepsMap[step],
				varsVar, varVal, -1)
		}
	}

	env, err := getIniEnv(stepsMap["env"])
	errExit(err, fmt.Sprintf("incorrect env %s", iniFile))

	steps.env = r.prepareEnv(env, p, pc)
	steps.buildDir = fp.Join(pc.tmpDir, src.dirName)

	// replace distro defined vars in each step (replaces <build_dir>)
	for step, val := range stepsMap {
		stepsMap[step] = r.replaceIniVars(val, p, pc, src, steps)
	}

	steps.prepare = stepsMap["prepare"]
	steps.configure = stepsMap["configure"]
	steps.build = stepsMap["build"]
	steps.pkg_create = stepsMap["pkg_create"]

	for _, step := range subPkgList {
		var subPkg subPkgT
		subPkg.suffix = str.TrimPrefix(step, "subpkg_")
		subPkg.files = str.Split(stepsMap[step], " ")
		steps.subPkgs = append(steps.subPkgs, subPkg)
	}

	return src, steps, false
}

func lineIsSubPkgDef(line, set, section string) bool {
	setFields := str.Split(set, "_")
	subName := "subpkg_" + setFields[len(setFields)-1] + " = "

	return str.HasPrefix(set, section+"_") && str.HasPrefix(line, subName)
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

	if str.HasPrefix(line, "subpkg_") && str.Contains(line, " = ") {
		return true
	}

	return false
}

func (r *runT) replaceIniVars(s string, p pkgT, pc pkgCfgT, src srcT,
	steps stepsT) string {
	replMap := r.setReplMap(p, pc, src, steps)

	for k, v := range replMap {
		if v != "" {
			s = str.Replace(s, k, v, -1)
		}
	}

	return s
}

func (r *runT) setReplMap(p pkgT, pc pkgCfgT, src srcT, steps stepsT) map[string]string {
	return map[string]string{
		"<root_dir>":    r.rootDir,
		"<prog>":        p.prog,
		"<ver>":         p.ver,
		"<ver_short>":   p.verShort,
		"<pkg_rel>":     p.newRel,
		"<set_ver_rel>": p.setVerNewRel,
		"<pkg_dir>":     p.newPkgDir,
		"<prog_dir>":    p.progDir,
		"<patch_dir>":   p.patchDir,
		"<src_dir>":     fp.Join(p.progDir, "src"),
		"<ver_pkgspec>": getPkgSpecVer(p),
		"<src_path>":    src.file,
		"<tmp_dir>":     pc.tmpDir,
		"<build_dir>":   steps.buildDir,
	}
}

func getPkgSpecVer(p pkgT) string {
	var v string

	switch p.prog {
	case "sqlite":
		v = str.Replace(p.ver, ".", "", -1) + "000"
	case "libnl":
		v = str.Replace(p.ver, ".", "_", -1)
	case "cdrtools":
		v = str.Split(p.ver, "a")[0]
	case "c-ares":
		v = str.Replace(p.ver, ".", "_", -1)
	case "doxygen":
		v = str.Replace(p.ver, ".", "_", -1)
	case "libcdio-paranoia":
		v = str.Replace(p.ver, "+", "-", -1)
	case "boost":
		v = str.Replace(p.ver, ".", "_", -1)
	case "libexif":
		v = str.Replace(p.ver, ".", "_", -1)
	case "vim":
		vSplit := str.Split(p.ver, ".")
		v = vSplit[0] + vSplit[1]
	case "w3m":
		v = str.Replace(p.ver, "+git", "-git", -1)
	case "unzip":
		v = str.Replace(p.ver, ".", "", -1)
	case "fetchmail":
		v = str.Replace(p.ver, ".", "-", -1)
	case "zip":
		v = str.Replace(p.ver, ".", "", -1)
	case "tinyxml":
		v = str.Replace(p.ver, ".", "_", -1)
	}

	return v
}

func getRegexes() reT {
	var re reT

	re.wSpaces = regexp.MustCompile(`\s+`)
	re.pkgName = regexp.MustCompile(`^[a-z0-9_]+/[\w-+]+$`)

	r := `^[0-9]{4}-[0-9]{2}-[0-9]{2}\.[a-z0-9]{8}$`
	re.gitVer = regexp.MustCompile(r)

	re.noNoSharedLib = regexp.MustCompile(`^/lib/lib.*\.so.*$`)
	re.noNoStaticLib = regexp.MustCompile(`^/usr/lib/lib.*\.a$`)
	re.staticBin = regexp.MustCompile(`^/s*bin/`)

	r = `(^/usr/s*bin/|^/usr/lib/lib.*\.so.*|^/usr/libexec)`
	re.glibcBin = regexp.MustCompile(r)

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

func (r *runT) getWorldPkgs(instDir string) []pkgT {
	var worldPkgs []pkgT
	pkgVerMap := make(map[string]string)

	worldDir := fp.Join(instDir, "/var/xx")
	if !fileExists(worldDir) {
		return worldPkgs
	}

	dirs, err := walkDir(worldDir, "dirs")
	errExit(err, "can't walk world dir: "+worldDir)

	for _, dir := range dirs {
		d := str.TrimPrefix(dir, worldDir+"/")
		fields := str.Split(d, "/")
		if len(fields) != 3 || str.HasSuffix(dir, "/var/xx") {
			continue
		}
		name, pkgSetVerRel := fields[0]+"/"+fields[1], fields[2]
		fields = str.Split(pkgSetVerRel, "-")
		if len(fields) < 3 {
			errExit(ERR, "can't parse line:", d)
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
		p := r.getPkg(name, set, ver)

		worldPkgs = append(worldPkgs, p)
	}

	return worldPkgs
}

//
func (r *runT) parseSharedLibsFile(sharedLibsFile string) []pkgT {
	var deps []pkgT

	fd, err := os.Open(sharedLibsFile)
	errExit(err, "can't open shared_libs file "+sharedLibsFile)
	defer fd.Close()
	input := bufio.NewScanner(fd)

	for input.Scan() {
		fields := str.Split(input.Text(), "\t")
		name := fields[1]
		pkgSet := fields[2]
		dep := r.getPkg(name, pkgSet, "latest")
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
		if cntDirName == "common" || str.HasPrefix(cntDirName, ".") {
			continue
		}
		cntList = append(cntList, cntDirName)
	}

	return cntList
}

// returns a map of config files to be installed with a pkg
// file in root dir -> location of the file
func (r *runT) getPkgCfgFiles(p pkgT) map[string]string {
	cfgFiles := make(map[string]string)

	// get files from pkg cfg dir
	if fileExists(p.cfgDir) {
		files, err := walkDir(p.cfgDir, "files")
		errExit(err, "can't walk pkg cfg dir: "+p.cfgDir)
		for _, file := range files {
			rootFile := str.TrimPrefix(file, p.cfgDir)
			if file == rootFile {
				errExit(ERR, "can't copy from root dir")
			}
			cfgFiles[rootFile] = file
		}
	}

	// get files from system cfg dir for the pkg
	pkgSysCfgDir := findCfgDir(fp.Join(r.sysCfgDir, p.prog), p)
	if fileExists(pkgSysCfgDir) {
		files, err := walkDir(pkgSysCfgDir, "files")
		errExit(err, "can't walk sys cfg dir: "+pkgSysCfgDir)
		for _, file := range files {
			rootFile := str.TrimPrefix(file, pkgSysCfgDir)
			if file == rootFile {
				errExit(ERR, "can't copy from root dir")
			}
			cfgFiles[rootFile] = file
		}
	}

	return cfgFiles
}
