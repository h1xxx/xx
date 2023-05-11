package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"

	fp "path/filepath"
	str "strings"
)

func (r *runT) actionInfo() {

	switch {
	// todo: move to argsCheck
	case r.rootDir == "":
		errExit(errors.New(""), "root dir argument (-r) missing")

	case r.infoDeps:
		r.printDepsInfo(r.pkgs, r.pkgCfgs)

	case r.infoInteg:
		r.sysVerify(r.pkgs, r.pkgCfgs)
	}
}

func (r *runT) printDepsInfo(pkgs []pkgT, pkgCfgs []pkgCfgT) {
	for i, p := range pkgs {
		fmt.Printf("+ %-32s %s\n", p.name, p.setVerRel)
		pc := pkgCfgs[i]

		if len(pc.runDeps) > 0 {
			fmt.Println("\nrun-time dependencies:")
		}
		for _, d := range pc.runDeps {
			fmt.Printf("  %-32s %s\n", d.name, d.setVerRel)
		}

		if len(pc.libDeps) > 0 {
			fmt.Println("\nshared libs:")
		}
		for _, dep := range pc.libDeps {
			fmt.Printf("  %-32s %s\n", dep.name, dep.setVerRel)
		}

		// todo : add flags to activate this
		/*
			fmt.Println("\nrun-time dependencies tree:")
			r.printPkgDepsTree(p, pc.runDeps, []pkgT{}, "run", 1)

			fmt.Println("\nshared libs tree:")
			r.printPkgDepsTree(p, pc.libDeps, []pkgT{}, "lib", 1)

			fmt.Println("\nfull dependency tree:")
			r.printPkgDepsTree(p, pc.allRunDeps, []pkgT{}, "all", 1)
		*/

		// todo: add build deps

		deps := r.getAllDeps(p, pc.allRunDeps, []pkgT{},
			"all", 1)
		sort.Slice(deps, func(i, j int) bool {
			return deps[i].name <= deps[j].name
		})
		if len(deps) > 0 {
			fmt.Println("\nall deps:")
		}
		for _, dep := range deps {
			fmt.Printf("  %-32s %s\n", dep.name, dep.setVerRel)
		}

		fmt.Println()
	}
}

// prints recursively all pkg dependencies
// depType possible vaules: "all", "run", "lib", "build"
func (r *runT) printPkgDepsTree(p pkgT, deps, topPkgs []pkgT, depType string, depth int) {
	if pkgExists(p, topPkgs) {
		return
	}

	topPkgs = append(topPkgs, p)

	indent := "  "
	for i := 1; i < depth; i++ {
		indent += "    "
	}

	// calculates the column where to start printing pkg set-ver
	col := fmt.Sprintf("%d", 56-4*depth)

	for _, dep := range deps {
		if dep.name == "dev/glibc" && depth != 1 {
			continue
		}

		fmt.Printf("%s%-"+col+"s %s\n", indent, dep.name, dep.setVerRel)

		var depDeps []pkgT
		depC := r.getPkgCfg(dep, "")
		switch depType {
		case "all":
			depDeps = depC.allRunDeps
		case "run":
			depDeps = depC.runDeps
		case "lib":
			depDeps = depC.libDeps
		case "build":
			depDeps = depC.buildDeps
		}

		if p.name != dep.name {
			r.printPkgDepsTree(dep, depDeps, topPkgs, depType, depth+1)
		}
	}
}

func (r *runT) sysVerify(pkgs []pkgT, pkgCfgs []pkgCfgT) {

	var filesMod, filesNew, filesOK, filesMissing []string

	fmt.Println("getting a list of system files...")
	sysFiles, err := walkDir(r.rootDir, "sysfiles")
	errExit(err, "couldn't list all files in:\n  "+r.rootDir)

	sort.Slice(sysFiles, func(i, j int) bool {
		return sysFiles[i] <= sysFiles[j]
	})

	fmt.Println("getting a list of files for each p...")
	fileHash, filePkg, dupes := r.getFileMaps(pkgs, pkgCfgs)

	// make sure that there are no duplicate files
	if len(dupes) != 0 {
		fmt.Println("* duplicate files:\n")
		for _, dup := range dupes {
			fmt.Println(dup)
		}
		errExit(errors.New(""), "duplicate files in pkgs need fixing")
	}

	fmt.Println("looking for new and changed files...")
	for _, file := range sysFiles {

		sum := getFileHash(file)
		if r.rootDir != "/" {
			file = "/" + str.TrimPrefix(file, r.rootDir)
		}

		_, keyExists := fileHash[file]
		if !keyExists {
			filesNew = append(filesNew, file)
		} else if sum != fileHash[file] {
			filesMod = append(filesMod, filePkg[file].name+"\t"+file)
		} else if sum == fileHash[file] {
			filesOK = append(filesOK, file)
		}
	}

	fmt.Println("looking for the missing files...")
	for f, p := range filePkg {
		sysFile := fp.Clean(r.rootDir + "/" + f)
		idx := sort.SearchStrings(sysFiles, sysFile)

		if idx > len(sysFiles)-1 || sysFiles[idx] != sysFile {
			filesMissing = append(filesMissing, p.name+"\t"+f)
		}
	}

	if len(filesNew) != 0 {
		fmt.Println("\nnew files:\n")
		for _, f := range filesNew {
			fmt.Println(f)
		}
	}

	if len(filesMod) != 0 {
		fmt.Println("\nmodified files:\n")
		for _, f := range filesMod {
			fmt.Println(f)
		}
	}

	if len(filesMissing) != 0 {
		fmt.Println("\nmissing files:\n")
		for _, f := range filesMissing {
			fmt.Println(f)
		}
	}

	fmt.Printf("%-16s %d\n", "\nsystem files: ", len(sysFiles))
	fmt.Printf("%-16s %d\n", "new files: ", len(filesNew))
	fmt.Printf("%-16s %d\n", "modified files: ", len(filesMod))
	fmt.Printf("%-16s %d\n", "missing files: ", len(filesMissing))

}

func (r *runT) getFileMaps(pkgs []pkgT, pkgCfgs []pkgCfgT) (map[string]string, map[string]pkgT, []string) {

	fileHash := make(map[string]string)
	filePkg := make(map[string]pkgT)
	var dupes []string

	for i, p := range pkgs {
		// get hashes for each file in the pkg from the logs
		_, fh := getPkgFiles(p)

		// add hashes of pkg config files
		if fileExists(p.cfgDir) {
			cfgFiles, err := walkDir(p.cfgDir, "files")
			errExit(err, "couldn't list all files in:\n  "+
				r.rootDir)

			for _, f := range cfgFiles {
				h := getFileHash(f)
				f = str.TrimPrefix(f, p.cfgDir)
				fh[f] = h
			}
		}

		// add hashes of system config files
		cfgSysPkgDir := r.sysCfgDir + "/" + p.prog + "/" +
			p.set + "-" + p.ver

		if !fileExists(cfgSysPkgDir) {
			cfgSysPkgDir = r.sysCfgDir + "/" + p.prog + "/" +
				p.set + "-latest"
		}

		if r.sysCfgDir != "" && fileExists(cfgSysPkgDir) {
			cfgFiles, err := walkDir(cfgSysPkgDir, "files")
			errExit(err, "couldn't list all files in:\n  "+
				cfgSysPkgDir)

			for _, f := range cfgFiles {
				h := getFileHash(f)
				f = str.TrimPrefix(f, cfgSysPkgDir)
				fh[f] = h
			}
		}

		// store files and hashes in maps for all packages
		for f, h := range fh {
			if pkgCfgs[i].cnt {
				f = "/cnt/rootfs/" + p.prog + "/" + f
			}
			f = fp.Clean(f)

			_, keyExists := fileHash[f]
			if keyExists {
				dup := f + "\n\t" + p.name +
					"\n\t" + filePkg[f].name
				dupes = append(dupes, dup)
			}
			fileHash[f] = h
			filePkg[f] = p
		}
	}

	return fileHash, filePkg, dupes
}

func getFileHash(file string) string {
	fd, err := os.Open(file)
	errExit(err, "can't open file: "+file)

	hash := sha256.New()
	_, err = io.Copy(hash, fd)
	errExit(err, "can't read file: "+file)
	fd.Close()

	sum := hex.EncodeToString(hash.Sum(nil))

	return sum
}
