package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"time"

	fp "path/filepath"
	str "strings"
)

func (r *runT) actionDiff() {
	r.diffPkgs(r.pkgs, r.pkgCfgs)
}

func (r *runT) diffPkgs(pkgs []pkgT, pkgCfgs []pkgCfgT) {
	for i, p := range pkgs {
		r.diffPkg(p, pkgCfgs[i])
	}
}

func (r *runT) diffPkg(p pkgT, pc pkgCfgT) {
	if r.isOldBuild(p) {
		return
	}

	pkgPrev := r.getPkgPrev(p)
	pkgCPrev := r.getPkgCfg(pkgPrev, "")

	if p == pkgPrev {
		return
	}

	_, fileHash := getPkgFiles(p)
	_, fileHashPrev := getPkgFiles(pkgPrev)

	// results map: map[<category>][]<list of files>
	// categories:  "changed", "new", "removed", "etc", "config",
	// 		"removed_libs", "new_libs"
	res, etcFiles := getRes(fileHash, fileHashPrev)
	getEtcDiff(etcFiles, p, res)
	getConfigDiff(p, res)
	getDirStatus(p, pkgPrev, res)
	getLibsDiff(pc, pkgCPrev, fileHash, fileHashPrev, res)

	printDiffRes(res)
}

func (r *runT) isOldBuild(p pkgT) bool {
	// time since last build in hours
	shaLog := fp.Join(p.progDir, "log", p.setVerRel, "sha256.log")
	stats, err := os.Stat(shaLog)
	if err != nil {
		return false
	}
	timeDiff := (time.Now().Unix() - stats.ModTime().Unix()) / 60 / 60

	return timeDiff > r.diffHours
}

func (r *runT) getPkgPrev(p pkgT) pkgT {
	pkgPrev := p
	if r.diffBuild {
		pkgPrev.setVerRel = p.setVerPrevRel
		if p.setVerRel == p.setVerPrevRel {
			return pkgPrev
		}
		fmt.Printf("\n\n+ %-32s %s => %s\n", p.name,
			p.setVerPrevRel, p.setVerRel)
	} else {
		return getPkgPrevVer(p)
	}
	return pkgPrev
}

func getPkgPrevVer(p pkgT) pkgT {
	var versions []string

	dirs, err := os.ReadDir(fp.Join(p.progDir, "pkg"))
	errExit(err, "can't open package dir")

	for _, dir := range dirs {
		if str.HasPrefix(dir.Name(), p.set+"-") {
			var ver, sep string

			fields := str.Split(dir.Name(), "-")
			if len(fields) < 3 {
				msg := "can't extract ver from %s"
				errExit(fmt.Errorf(msg, dir.Name()), "")
			}

			verRaw := str.Join(fields[1:len(fields)-1], "-")
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
		errExit(errors.New(""), "no pkg dirs available for "+p.name)
	}

	verIdx := len(versions) - 1
	if len(versions) > 1 {
		verIdx = len(versions) - 2
	}

	p.ver = str.Replace(versions[verIdx], " ", "", -1)
	p.verShort = getVerShort(p.ver)
	p.rel, p.prevRel, p.newRel = getPkgRels(p)
	p = getPkgSetVers(p)
	p = getPkgDirs(p)

	return p
}

func getRes(fileHash, fileHashPrev map[string]string) (map[string][]string,
	[]string) {

	res := make(map[string][]string)
	var etcFiles []string

	for file, hash := range fileHash {
		prevHash, exists := fileHashPrev[file]
		if !exists {
			res["new"] = append(res["new"], file)
			continue
		}
		if hash != prevHash {
			res["changed"] = append(res["changed"], file)
			if str.Contains(file, "/etc/") ||
				str.HasSuffix(file, ".conf") {

				etcFiles = append(etcFiles, file)
			}
		}
	}

	for filePrev := range fileHashPrev {
		_, exists := fileHash[filePrev]
		if !exists {
			res["removed"] = append(res["removed"], filePrev)
			continue
		}
	}
	sort.Strings(res["changed"])
	sort.Strings(res["new"])
	sort.Strings(res["removed"])

	return res, etcFiles
}

func getDiff(file1, file2 string) ([]string, bool) {
	var change bool

	argsSlice := []string{"--no-pager", "diff", "--no-index",
		"--exit-code", "--color=always", file1, file2}

	cmd := exec.Command("git", argsSlice...)
	diffOut, err := cmd.Output()
	exitErr, exitNot0 := err.(*exec.ExitError)

	switch {
	case exitNot0:
		change = true
	case !exitNot0:
		change = false
	case exitErr.ExitCode() == 1:
		change = true
	default:
		errExit(errors.New(""), "can't get a diff")
	}
	diff := str.Split(string(diffOut), "\n")

	return diff, change
}

func getEtcDiff(etcFiles []string, p pkgT, res map[string][]string) {
	for i, f := range etcFiles {
		diff, _ := getDiff(fp.Join(p.prevPkgDir, f),
			fp.Join(p.pkgDir, f))

		res["etc"] = append(res["etc"], f+"\n")
		for _, line := range diff {
			if !skipLine(line) {
				res["etc"][i] += line + "\n"
			}
		}
	}
	sort.Strings(res["etc"])
}

func skipLine(line string) bool {
	prefixes := []string{"\033[1m--- a/", "\033[1m+++ b/",
		"\033[1mdiff --git a/", "\033[1mold mode ",
		"\033[1mnew mode ", "\033[1mindex "}

	for _, p := range prefixes {
		if str.HasPrefix(line, p) {
			return true
		}
	}

	return false
}

func getConfigDiff(p pkgT, res map[string][]string) {
	configPrevLog := fp.Join(p.progDir, "log", p.setVerPrevRel,
		"config-help.log")
	configLog := fp.Join(p.progDir, "log", p.setVerRel,
		"config-help.log")

	if !fileExists(configPrevLog) || !fileExists(configLog) {
		return
	}

	diff, change := getDiff(configPrevLog, configLog)

	if !change || len(diff) == 0 {
		return
	}

	for _, line := range diff {
		if !skipLine(line) {
			res["config"] = append(res["config"], line+"\n")
		}
	}
}

func getDirStatus(p, pkgPrev pkgT, res map[string][]string) {
	dirStat := make(map[string]int64)
	dirStatPrev := make(map[string]int64)

	dirCount := make(map[string]int64)
	dirCountPrev := make(map[string]int64)
	changeCount := make(map[string]int64)
	changeCountPrev := make(map[string]int64)

	files, err := walkDir(p.pkgDir, "files")
	errExit(err, "can't get a list of files in: "+p.pkgDir)

	filesPrev, err := walkDir(pkgPrev.pkgDir, "files")
	errExit(err, "can't get a list of files in: "+pkgPrev.pkgDir)

	getDirStats(dirStat, dirCount, changeCount, files, p, res)
	getDirStats(dirStatPrev, dirCountPrev, changeCountPrev, filesPrev,
		pkgPrev, res)

	fmtStr := "%-25s %4d %8d %8d %12s %8s"

	for dir, size := range dirStat {
		sizePrev := dirStatPrev[dir]
		line := fmt.Sprintf(fmtStr, dir,
			dirCountPrev[dir], dirCount[dir],
			changeCount[dir], intFormat(sizePrev), intFormat(size))
		res["dir_status"] = append(res["dir_status"], line)
	}

	sort.Strings(res["dir_status"])

	for dirPrev, sizePrev := range dirStatPrev {
		size, exists := dirStat[dirPrev]
		if exists {
			continue
		}
		line := fmt.Sprintf(fmtStr, dirPrev,
			dirCountPrev[dirPrev], dirCount[dirPrev],
			changeCount[dirPrev], intFormat(sizePrev),
			intFormat(size))
		res["dir_status"] = append(res["dir_status"], line)
	}
}

func getDirStats(dirStat, dirCount, changeCount map[string]int64,
	files []string, p pkgT, res map[string][]string) {

	for _, file := range files {
		fullDir := fp.Dir(file)
		dir := "/" + str.TrimPrefix(fullDir, p.pkgDir)
		dirS := str.Split(dir, "/")
		d := str.Join(dirS[:min(4, len(dirS))], "/")

		dirCount[d] += 1
		if fileIsChanged(dir+"/"+fp.Base(file), res) {
			changeCount[d] += 1
		}

		_, exists := dirStat[d]
		if exists {
			continue
		} else {
			size := dirSize(fullDir)
			dirStat[d] = dirStat[d] + size/1024
		}
	}
}

func getLibsDiff(pc, pkgCPrev pkgCfgT, fileHash, fileHashPrev map[string]string, res map[string][]string) {
	var files, filesPrev []string
	for file := range fileHash {
		if str.HasPrefix(file, "/usr/include") ||
			str.HasPrefix(file, "/usr/share/terminfo") ||
			str.HasPrefix(file, "/usr/share/man") ||
			str.HasPrefix(file, "/usr/share/doc") ||
			str.HasPrefix(file, "/usr/share/locale") {
			continue
		}
		files = append(files, file)
	}
	for file := range fileHashPrev {
		if str.HasPrefix(file, "/usr/include") ||
			str.HasPrefix(file, "/usr/share/terminfo") ||
			str.HasPrefix(file, "/usr/share/man") ||
			str.HasPrefix(file, "/usr/share/doc") ||
			str.HasPrefix(file, "/usr/share/locale") {
			continue
		}
		filesPrev = append(filesPrev, file)
	}

	pkgDeps := pc.libDeps
	pkgDepsPrev := pkgCPrev.libDeps

	for _, libPkg := range pkgDeps {
		if !pkgExists(libPkg, pkgDepsPrev) {
			res["new_libs"] = append(res["new_libs"], libPkg.name)
		}
	}

	for _, libPkg := range pkgDepsPrev {
		if !pkgExists(libPkg, pkgDeps) {
			res["removed_libs"] = append(res["removed_libs"], libPkg.name)
		}
	}

	// todo: add names of the libary files
}

func fileIsChanged(file string, res map[string][]string) bool {
	for _, f := range res["changed"] {
		if f == file {
			return true
		}
	}
	return false
}

func dirSize(path string) int64 {
	var size int64
	err := fp.Walk(path, func(_ string, info os.FileInfo,
		err error) error {

		if err != nil {
			return err
		}
		if !info.IsDir() && !(info.Mode()&os.ModeSymlink != 0) {
			size += info.Size()
		}
		return err
	})
	errExit(err, "can't get dir size: "+path)

	return size
}

func printDiffRes(res map[string][]string) {
	fmt.Println("\n\t\t\t\tfile count\t\t  size (KB)")
	fmt.Println("\t\t      previous\tcurrent\t changed     previous  current")
	fmt.Println(str.Join(res["dir_status"], "\n"), "\n")

	if len(res["new"]) > 0 {
		fmt.Printf("new:\n%s\n\n",
			str.Join(res["new"], "\n"))
	}

	if len(res["removed"]) > 0 {
		fmt.Printf("removed:\n%s\n\n",
			str.Join(res["removed"], "\n"))
	}

	if len(res["new_libs"]) > 0 {
		fmt.Printf("new libs:\n%s\n\n",
			str.Join(res["new_libs"], "\n"))
	}

	if len(res["removed_libs"]) > 0 {
		fmt.Printf("removed libs:\n%s\n\n",
			str.Join(res["removed_libs"], "\n"))
	}

	if len(res["config"]) > 0 {
		fmt.Println("diff of configure help:\n")
		fmt.Println(str.Join(res["config"], ""))
	}

	if len(res["etc"]) > 0 {
		fmt.Println("diff of files in '/etc/':\n")
		fmt.Println(str.Join(res["etc"], ""))
	}

}

func intFormat(n int64) string {
	in := strconv.FormatInt(n, 10)
	numOfDigits := len(in)
	if n < 0 {
		numOfDigits-- // First character is the - sign (not a digit)
	}
	numOfCommas := (numOfDigits - 1) / 3

	out := make([]byte, len(in)+numOfCommas)
	if n < 0 {
		in, out[0] = in[1:], '-'
	}

	for i, j, k := len(in)-1, len(out)-1, 0; ; i, j = i-1, j-1 {
		out[j] = in[i]
		if i == 0 {
			return string(out)
		}
		if k++; k == 3 {
			j, k = j-1, 0
			out[j] = ','
		}
	}
}
