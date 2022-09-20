package main

import (
	"errors"
	"fmt"
	"os"
	fp "path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"

	toml "github.com/pelletier/go-toml"
)

func checkConf(pkg pkgT, conf *toml.Tree) {
	var errMsg string
	var stepMissing bool

	srcVars := []string{"url", "src_type", "src_dirname"}
	stepVars := []string{"env", "prepare", "configure", "build",
		"pkg_create"}

	if !conf.Has(pkg.set) {
		errMsg += "\tsection \"" + pkg.set + "\"\n"
		stepMissing = true
	}

	for _, v := range srcVars {
		if !conf.HasPath([]string{"src", v}) {
			errMsg += "\t\"" + v + "\" in section \"src\"\n"
		}
	}

	if !stepMissing {
		for _, v := range stepVars {
			if !conf.HasPath([]string{pkg.set, v}) {
				errMsg += "\t\"" + v + "\" in section \"" +
					pkg.set + "\"\n"
			}
		}
	}

	if errMsg != "" {
		errExit(errors.New(""),
			"missing variables in .toml file:\n"+pkg.name+errMsg)
	}
}

// replaces a string with another one in-place
func repl(s *string, a string, b string) {
	*s = strings.Replace(*s, a, b, -1)
}

func getVer(pkg pkgT, fixedVer string) string {
	if fixedVer != "" && fixedVer != "latest" {
		return fixedVer
	}

	var versions []string
	files, err := os.ReadDir(pkg.progDir)
	errExit(err, "can't open prog dir")

	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".toml") {
			var ver, sep string
			verRaw := strings.Split(file.Name(), ".toml")[0]
			verSplit := strings.Split(verRaw, ".")
			for _, v := range verSplit {
				ver += fmt.Sprintf("%s%32s", sep, v)
				sep = "."
			}
			versions = append(versions, ver)
		}
	}
	sort.Strings(versions)

	if len(versions) == 0 {
		errExit(errors.New(""), "no toml file available for "+pkg.name)
	}

	return strings.Replace(versions[len(versions)-1], " ", "", -1)
}

func getVerShort(ver string) string {
	var verShort string
	fields := strings.Split(ver, ".")
	l := len(fields)

	switch {
	case isGitVer(ver):
		verShort = fields[1]
	case l == 4:
		verShort = fields[0] + "." + fields[1] + "." + fields[2]
	case l == 3:
		verShort = fields[0] + "." + fields[1]
	case l == 2:
		verShort = fields[0]
	case l == 1:
		verShort = fields[0]
	}

	return verShort
}

func isGitVer(ver string) bool {
	re := regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}\.[a-z0-9]{8}$`)
	return re.MatchString(ver)
}

func getBuildEnv(actionTarget string) string {
	if !isPkgString(actionTarget) {
		file := fp.Base(actionTarget)
		fields := strings.Split(file, ".")
		buildEnv := strings.Join(fields[:len(fields)-1], ".")
		switch {
		case buildEnv == "bootstrap-base":
			buildEnv = "bootstrap"
		case buildEnv == "":
			buildEnv = "base"
		}
		return buildEnv
	} else {
		return strings.Split(actionTarget, "/")[1]
	}
}

func getPkgRels(pkg pkgT) (string, string, string) {
	var id int64
	progPkgDir := fp.Join(pkg.progDir, "pkg")

	// return "00" if the pkg dir is empty
	if !fileExists(progPkgDir) || dirIsEmpty(progPkgDir) {
		return "00", "00", "00"
	}

	if pkg.setVerRel == "" {
		id = getLastRel(progPkgDir, pkg.set+"-"+pkg.ver+"-")
	} else {
		var err error
		idSplit := strings.Split(pkg.setVerRel, "-")
		id, err = strconv.ParseInt(idSplit[len(idSplit)-1], 16, 64)
		errExit(err, "unable to convert pkg release")
	}

	pkgRel := fmt.Sprintf("%0.2x", id)
	pkgPrevRel := fmt.Sprintf("%0.2x", max(0, int(id-1)))
	pkgNewRel := fmt.Sprintf("%0.2x", id+1)

	if id+1 > 255 {
		errExit(errors.New(""), "max pkgRel reached")
	}

	// return "00" if dir of the pkg to build doesn't exist
	pkgToBuild := fp.Join(progPkgDir, pkg.set+"-"+pkg.ver+"-"+pkgRel)
	if !fileExists(pkgToBuild) {
		return "00", "00", "00"
	}

	return pkgRel, pkgPrevRel, pkgNewRel
}

func getLastRel(pkgDir, dirPrefix string) int64 {
	var id int64

	dirs, err := os.ReadDir(pkgDir)
	errExit(err, "can't open package dir")

	for _, dir := range dirs {
		if strings.HasPrefix(dir.Name(), dirPrefix) {
			s := strings.Split(dir.Name(), "-")
			idStr := s[len(s)-1]
			id, err = strconv.ParseInt(idStr, 16, 64)
			errExit(err, "unable to convert pkg release")
		}
	}
	return id
}

func prepareEnv(envIn []string, genC genCfgT, pkgC pkgCfgT) []string {
	var envOut []string
	envMap := make(map[string]string)

	if pkgC.crossBuild {
		envMap["PATH"] = genC.rootDir
		envMap["PATH"] += "/tools/bin:/bin:/sbin:/usr/bin:/usr/sbin"
		envMap["TARGET_TRIPLET"] = "x86_64-xx-linux-gnu"
	} else {
		envMap["PATH"] = "/bin:/sbin:/usr/bin:/usr/sbin"
		envMap["TARGET_TRIPLET"] = "x86_64-pc-linux-gnu"
	}

	if !pkgC.crossBuild {
		envMap["CFLAGS"] = "-O2 -pipe -fpie -fPIE " +
			"-fstack-protector-strong " +
			"-fstack-clash-protection -Wformat " +
			"-Wformat-security -D_FORTIFY_SOURCE=2"
		envMap["CXXFLAGS"] = envMap["CFLAGS"]
		envMap["LDFLAGS"] = "-Wl,-z,now -Wl,-z,relro -Wl,-z,noexecstack"
	}

	envMap["LC_ALL"] = "C"
	envMap["HOME"] = "/home/xx"
	envMap["USER"] = "xx"
	envMap["MAKEFLAGS"] = fmt.Sprintf("j%d", runtime.NumCPU())

	for _, e := range envIn {
		var add bool
		s := strings.Split(e, "=")
		key, val := s[0], strings.Join(s[1:], "=")
		if strings.HasSuffix(key, "+") {
			key = strings.TrimSuffix(key, "+")
			add = true
		}
		if val == "" {
			delete(envMap, key)
		} else if add {
			envMap[key] += " " + val
		} else {
			envMap[key] = val
		}
	}

	for k, v := range envMap {
		envOut = append(envOut, k+"="+v)
	}

	return envOut
}
