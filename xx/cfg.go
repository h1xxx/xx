package main

import (
	"fmt"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strconv"

	fp "path/filepath"
	str "strings"
)

// replaces a string with another one in-place
func repl(s *string, a string, b string) {
	*s = str.Replace(*s, a, b, -1)
}

func getVer(p pkgT, fixedVer string) string {
	if fixedVer != "" && fixedVer != "latest" {
		return fixedVer
	}

	var versions []string
	files, err := os.ReadDir(p.progDir)
	errExit(err, "can't open prog dir")

	for _, file := range files {
		if str.HasSuffix(file.Name(), ".ini") {
			var ver, sep string
			verRaw, _, _ := str.Cut(file.Name(), ".ini")
			verFields := str.Split(verRaw, ".")
			for _, v := range verFields {
				ver += fmt.Sprintf("%s%32s", sep, v)
				sep = "."
			}
			versions = append(versions, ver)
		}
	}
	sort.Strings(versions)

	if len(versions) == 0 {
		errExit(ERR, "no ini file available for", p.name)
	}

	return str.Replace(versions[len(versions)-1], " ", "", -1)
}

func getVerShort(ver string, re *regexp.Regexp) string {
	var verShort string
	fields := str.Split(ver, ".")
	l := len(fields)

	switch {
	case isGitVer(ver, re):
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

func isGitVer(ver string, re *regexp.Regexp) bool {
	return re.MatchString(ver)
}

// get new release from pkg dir if p.setVerRel is empty;
// get a release from setVerRel otherwise
func getPkgRels(p pkgT) (string, string, string) {
	var id int64
	progPkgDir := fp.Join(p.progDir, "pkg")

	// return "00" if the pkg dir is empty
	if !fileExists(progPkgDir) || dirIsEmpty(progPkgDir) {
		return "00", "00", "00"
	}

	if p.setVerRel == "" {
		id = getLastRel(progPkgDir, p.set+"-"+p.ver+"-")
	} else {
		var err error
		idSplit := str.Split(p.setVerRel, "-")
		id, err = strconv.ParseInt(idSplit[len(idSplit)-1], 16, 64)
		errExit(err, "unable to convert pkg release: "+p.name)
	}

	pkgRel := fmt.Sprintf("%0.2x", id)
	pkgPrevRel := fmt.Sprintf("%0.2x", max(0, int(id-1)))
	pkgNewRel := fmt.Sprintf("%0.2x", id+1)

	if id+1 > 255 {
		errExit(ERR, "max pkgRel reached")
	}

	// return "00" if dir of the pkg to build doesn't exist
	pkgToBuild := fp.Join(progPkgDir, p.set+"-"+p.ver+"-"+pkgRel)
	if !fileExists(pkgToBuild) {
		return "00", "00", "00"
	}

	return pkgRel, pkgPrevRel, pkgNewRel
}

func getLastRel(pkgDir, dirPrefix string) int64 {
	var id int64

	dirs, err := os.ReadDir(pkgDir)
	if err != nil {
		return id
	}

	for _, dir := range dirs {
		if str.HasPrefix(dir.Name(), dirPrefix) {
			s := str.Split(dir.Name(), "-")
			idStr := s[len(s)-1]
			id, err = strconv.ParseInt(idStr, 16, 64)
			errExit(err, "unable to convert pkg release: "+pkgDir)
		}
	}
	return id
}

func (r *runT) prepareEnv(envIn []string, p pkgT, pc pkgCfgT) []string {
	var envOut []string
	envMap := make(map[string]string)

	switch {
	case !pc.muslBuild && pc.crossBuild:
		envMap["PATH"] = r.rootDir
		envMap["PATH"] += "/tools/bin:/bin:/sbin:/usr/bin:/usr/sbin"
		envMap["TARGET_TRIPLET"] = "x86_64-xx-linux-gnu"

	case !pc.muslBuild:
		envMap["PATH"] = "/bin:/sbin"
		envMap["TARGET_TRIPLET"] = "x86_64-pc-linux-gnu"

	case r.buildEnv == "init_musl" && pc.crossBuild:
		envMap["PATH"] = r.rootDir + "/tools/bin"
		envMap["PATH"] += ":" + r.rootDir + "/cross_tools/bin"
		envMap["PATH"] += ":/bin:/sbin"
		envMap["TARGET_TRIPLET"] = "x86_64-xx-linux-musl"

	case r.buildEnv == "init_musl" && !pc.crossBuild:
		envMap["PATH"] = "/bin:/sbin:/tools/bin"
		envMap["TARGET_TRIPLET"] = "x86_64-pc-linux-musl"

	case pc.muslBuild:
		envMap["PATH"] = "/bin:/sbin"
		envMap["TARGET_TRIPLET"] = "x86_64-pc-linux-musl"
	}

	if !pc.crossBuild || pc.muslBuild {
		envMap["CFLAGS"] = "-O2 -pipe -fPIE " +
			"-fstack-protector-strong " +
			"-fstack-clash-protection -Wformat " +
			"-Wformat-security -D_FORTIFY_SOURCE=2 "
		envMap["LDFLAGS"] += "-Wl,-z,now -Wl,-z,relro -Wl,-z,noexecstack "
	}

	if pc.muslBuild && !pc.crossBuild {
		envMap["CFLAGS"] += "-fPIC -static-pie -I/include -Wl,-static "
		envMap["LDFLAGS"] += "-Wl,-static "
	}
	envMap["LDFLAGS"] += "-Wl,--verbose "
	envMap["CFLAGS"] += "-Wl,--verbose "
	envMap["CXXFLAGS"] = envMap["CFLAGS"]

	if pc.muslBuild && !pc.crossBuild {
		envMap["PKG_CONFIG_PATH"] = "/lib/pkgconfig"
	} else if !pc.crossBuild {
		envMap["PKG_CONFIG_PATH"] = "/usr/lib/pkgconfig"
	}

	envMap["PWD"] = r.rootDir
	envMap["LC_ALL"] = "C"
	envMap["HOME"] = "/home/xx"
	envMap["USER"] = "xx"
	envMap["MAKEFLAGS"] = fmt.Sprintf("j%d", runtime.NumCPU())

	for _, e := range envIn {
		var add bool
		s := str.Split(e, "=")
		key, val := s[0], str.Join(s[1:], "=")
		val = str.Trim(val, "\"")
		val = str.Trim(val, "'")
		if str.HasSuffix(key, "+") {
			key = str.TrimSuffix(key, "+")
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
