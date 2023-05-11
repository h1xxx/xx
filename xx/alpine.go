package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"

	fp "path/filepath"
	str "strings"
)

func (r *runT) getAlpinePkgs(p pkgT, pc pkgCfgT) {
	repos := r.getAlpineRepos(p, pc)
	rootFile := getAlpineRoot(p)
	createApkRoot(rootFile, pc.steps.buildDir, repos)

	r.instLxcConfig(p, pc)

	pkgMap := makeAlpinePkgMap(p, pc, repos)
	for repoName, url := range pkgMap {
		repo := str.Split(repoName, "/")[0]
		repoDir := fp.Dir(repos[repo])

		fName := path.Base(url)
		fPath := fp.Join(p.progDir, "src", fName)
		if !fileExists(fPath) {
			fmt.Printf("  downloading %s...\n", fName)
			downloadToFile(url, fPath)
		}

		link := repoDir + "/" + fName
		if !fileExists(link) {
			err := os.Symlink(fPath, link)
			errExit(err, "can't create symlink in: "+link)
		}
	}
}

func createApkRoot(rootFile, buildDir string, repos map[string]string) {
	fmt.Printf("  creating alpine rootfs...\n")
	c := "mkdir -p " + buildDir + "/rootfs && "
	c += "cd " + buildDir + "/rootfs && "
	c += "tar xf " + rootFile + " && "
	c += "mkdir home/xx && "
	c += "echo '"
	for _, repoFile := range repos {
		repoSplit := str.Split(repoFile, "/")
		repo := str.Join(repoSplit[:len(repoSplit)-2], "/")
		c += repo + "\n"
	}
	c += "'> etc/apk/repositories"

	cmd := exec.Command("/home/xx/bin/busybox", "sh", "-c", c)
	out, err := cmd.CombinedOutput()
	errExit(err, "can't create apk root: "+buildDir+"\n"+string(out))
}

func getAlpineRoot(p pkgT) string {
	url := "http://dl-cdn.alpinelinux.org/alpine/edge/releases/x86_64/"
	file := "alpine-minirootfs-20220328-x86_64.tar.gz"

	fPath := fp.Join(p.progDir, "src", file)
	if !fileExists(fPath) {
		fmt.Printf("  downloading %s...\n", file)
		downloadToFile(url+file, fPath)
	}

	return fPath
}

func (r *runT) getAlpineRepos(p pkgT, pc pkgCfgT) map[string]string {
	urls := str.Split(pc.src.url, " ")
	repos := make(map[string]string)

	for i, url := range urls {
		urlSplit := str.Split(url, "/")
		if len(urlSplit) < 3 {
			errExit(errors.New(""), "incorrect repo url")
		}

		repoName := urlSplit[len(urlSplit)-3]
		repoDir := fp.Join(p.progDir, "src/repo_"+r.date,
			repoName, "x86_64")

		err := os.MkdirAll(repoDir, 0755)
		errExit(err, "couldn't create dir: "+repoDir)

		fName := path.Base(url)
		fPath := fp.Join(repoDir, fName)
		repos[repoName] = fPath

		if !fileExists(fPath) {
			fmt.Printf("  downloading %d: %s...\n", i, fName)
			downloadToFile(url, fPath)
		}
	}
	return repos
}

func makeAlpinePkgMap(p pkgT, pc pkgCfgT, repos map[string]string) map[string]string {
	fmt.Printf("  making deps list...\n")
	apkDot := getAlpinePkgVer(p.prog)
	apkDot += " " + runApkDot(p.prog)
	for _, s := range pc.steps.env {
		split := str.Split(s, "=")
		envVar := split[0]
		if envVar != "ADD_PROGS" {
			continue
		}

		addProgs := str.Split(split[1], " ")
		for _, addProg := range addProgs {
			apkDot += " " + getAlpinePkgVer(addProg)
			apkDot += " " + runApkDot(addProg)
		}
	}
	split := str.Split(str.Replace(apkDot, "\"", "", -1), " ")

	pkgMap := make(map[string]string)
	for _, dep := range split {
		if dep == "" {
			continue
		}
		name, ver := getAlpinePkgNameVer(dep)
		repo := getAlpinePkgRepo(name, ver, repos)
		url := "http://dl-cdn.alpinelinux.org/alpine/edge/"
		url += repo + "/x86_64/" + dep + ".apk"
		pkgMap[repo+"/"+name] = url
	}

	return pkgMap
}

func runApkDot(prog string) string {
	c := "lxc-execute -n xx -P /tmp/ -- sh -c \""
	c += "apk dot " + prog + " | grep -- '->' | "
	c += "sed -e 's/^  //' -e 's/ -> /\\n/g' -e 's/\\[/ /g' | "
	c += "cut -d' ' -f1 | tr '\n' ' '\""

	cmd := exec.Command("/home/xx/bin/busybox", "sh", "-c", c)
	out, err := cmd.CombinedOutput()
	errExit(err, "can't get deps via apk: \n"+string(out))

	return string(out)
}

func getAlpinePkgVer(prog string) string {
	c := "lxc-execute -n xx -P /tmp/ -- sh -c \""
	c += "apk list " + prog + " | cut -d' ' -f1 \""

	cmd := exec.Command("/home/xx/bin/busybox", "sh", "-c", c)
	out, err := cmd.CombinedOutput()
	errExit(err, "can't get deps via apk: \n"+string(out))

	return str.Replace(string(out), "\n", "", -1)
}

func getAlpinePkgRepo(name, ver string, repos map[string]string) string {
	for repo, repoFile := range repos {
		c := "tar xf " + repoFile + " APKINDEX -O | "
		c += "grep -q ^P:" + name + "$"
		cmd := exec.Command("/home/xx/bin/busybox", "sh", "-c", c)

		pkgNotExists := cmd.Run()
		if pkgNotExists == nil {
			return repo
		}
	}
	errExit(errors.New(""), "repo not found for pkg: "+name+"-"+ver)
	return ""
}

func getAlpinePkgNameVer(pkgName string) (string, string) {
	split := str.Split(pkgName, "-")
	if len(split) < 3 {
		errExit(errors.New(""), "incorrect alpine pkg name: "+pkgName)
	}
	name := str.Join(split[:len(split)-2], "-")
	ver := str.Join(split[len(split)-2:], "-")
	return name, ver
}
