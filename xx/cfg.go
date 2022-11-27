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
)

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
		if strings.HasSuffix(file.Name(), ".ini") {
			var ver, sep string
			verRaw := strings.Split(file.Name(), ".ini")[0]
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
		errExit(errors.New(""), "no ini file available for "+pkg.name)
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

// get new release from pkg dir if pkg.setVerRel is empty;
// get a release from setVerRel otherwise
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
		errExit(err, "unable to convert pkg release: "+pkg.name)
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
			errExit(err, "unable to convert pkg release: "+pkgDir)
		}
	}
	return id
}

func prepareEnv(envIn []string, genC genCfgT, pkg pkgT, pkgC pkgCfgT) []string {
	var envOut []string
	envMap := make(map[string]string)

	switch {
	case !pkgC.muslBuild && pkgC.crossBuild:
		envMap["PATH"] = genC.rootDir
		envMap["PATH"] += "/tools/bin:/bin:/sbin:/usr/bin:/usr/sbin"
		envMap["TARGET_TRIPLET"] = "x86_64-xx-linux-gnu"

	case !pkgC.muslBuild:
		envMap["PATH"] = "/bin:/sbin:/usr/bin:/usr/sbin"
		envMap["TARGET_TRIPLET"] = "x86_64-pc-linux-gnu"

	case genC.buildEnv == "init_musl":
		envMap["PATH"] = genC.rootDir + "/tools/bin"
		envMap["PATH"] += ":" + genC.rootDir + "/cross_tools/bin"
		envMap["PATH"] += ":/bin:/sbin:/usr/bin:/usr/sbin"
		envMap["TARGET_TRIPLET"] = "x86_64-xx-linux-musl"

	case pkgC.muslBuild:
		envMap["PATH"] = "/bin:/sbin"
		envMap["TARGET_TRIPLET"] = "x86_64-pc-linux-musl"
	}

	if !pkgC.crossBuild || pkgC.muslBuild {
		envMap["CFLAGS"] = "-O2 -pipe -fpie -fPIE " +
			"-fstack-protector-strong " +
			"-fstack-clash-protection -Wformat " +
			"-Wformat-security -D_FORTIFY_SOURCE=2"
		envMap["CXXFLAGS"] = envMap["CFLAGS"]
		envMap["LDFLAGS"] = "-Wl,-z,now -Wl,-z,relro -Wl,-z,noexecstack"
	}

	if pkgC.muslBuild && !pkgC.crossBuild {
		envMap["CFLAGS"] += " -static-pie"
	}

	if pkgC.muslBuild && !pkgC.crossBuild {
		envMap["PKG_CONFIG_PATH"] = "/lib/pkgconfig"
	} else if !pkgC.crossBuild {
		envMap["PKG_CONFIG_PATH"] = "/usr/lib/pkgconfig"
	}

	envMap["LC_ALL"] = "C"
	envMap["HOME"] = "/home/xx"
	envMap["USER"] = "xx"
	envMap["MAKEFLAGS"] = fmt.Sprintf("j%d", runtime.NumCPU())

	for _, e := range envIn {
		var add bool
		s := strings.Split(e, "=")
		key, val := s[0], strings.Join(s[1:], "=")
		val = strings.Trim(val, "\"")
		val = strings.Trim(val, "'")
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
