package main

import (
	"bufio"
	"crypto/sha256"
	"debug/elf"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path"
	fp "path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

func createPkg(world map[string]worldT, genC genCfgT, pkg pkgT, pkgC pkgCfgT) pkgT {
	if pkg.newRel != "00" && !pkgC.force || pkgC.src.srcType == "files" {
		return pkg
	}

	makeBuildDirs(pkg, pkgC)
	getSrc(genC, pkg, pkgC)

	execStep("prepare", genC, pkg, pkgC)
	execStep("configure", genC, pkg, pkgC)
	execStep("build", genC, pkg, pkgC)

	err := os.MkdirAll(pkg.newPkgDir, 0700)
	errExit(err, "couldn't create dir: "+pkg.newPkgDir)
	execStep("pkg_create", genC, pkg, pkgC)

	moveLogs(pkg, pkgC)
	saveHelp(genC, pkg, pkgC)
	cleanup(pkg, pkgC)
	dumpSHA256(pkg)

	if dirIsEmpty(pkg.newPkgDir) {
		fmt.Println("! pkg empty:", pkg.newPkgDir)
	}

	for _, s := range pkgC.steps.subPkgs {
		subPkg := getSubPkg(pkg, s.suffix)
		fmt.Printf("  creating subpkg %s...\n", subPkg.set)
		createSubPkg(pkg, subPkg, s.files)
	}

	// remove old pkg from world
	delete(world["/"].pkgs, pkg)
	for _, s := range pkgC.steps.subPkgs {
		subPkg := getSubPkg(pkg, s.suffix)
		delete(world["/"].pkgs, subPkg)
	}

	// get new release info after the build
	pkg.setVerRel = ""
	pkg.rel, pkg.prevRel, pkg.newRel = getPkgRels(pkg)
	pkg = getPkgSetVers(pkg)
	pkg = getPkgDirs(pkg)

	// add a new pkg and all subpkgs to root of the world;
	// no cnt here as only build step executes this
	addPkgToWorldT(world, pkg, "/")
	for _, s := range pkgC.steps.subPkgs {
		subPkg := getSubPkg(pkg, s.suffix)
		addPkgToWorldT(world, subPkg, "/")
	}

	// dump shared libs for main pkg and for subpkgs
	if !pkgC.crossBuild {
		dumpSharedLibs(world, genC, pkg)
	}
	for _, s := range pkgC.steps.subPkgs {
		subPkg := getSubPkg(pkg, s.suffix)
		dumpSharedLibs(world, genC, subPkg)
		selfLibsExist(world, genC, subPkg)
	}

	return pkg
}

func getSubPkg(pkg pkgT, suffix string) pkgT {
	subPkg := pkg
	subPkg.set = pkg.set + "_" + suffix
	subPkg = getPkgSetVers(subPkg)
	subPkg = getPkgDirs(subPkg)
	return subPkg
}

func createSubPkg(pkg, subPkg pkgT, files []string) {
	for _, f := range files {
		src := fp.Join(pkg.newPkgDir, f)
		dest := fp.Join(subPkg.newPkgDir, f)
		Mv(src, dest)
		RemEmptyDirs(fp.Dir(src))
		MoveShaInfo(pkg, subPkg, f)
	}
}

func MoveShaInfo(pkg, subPkg pkgT, file string) {
	src := fp.Join(pkg.progDir, "log", pkg.setVerNewRel, "sha256.log")
	dest := fp.Join(subPkg.progDir, "log", subPkg.setVerNewRel, "sha256.log")

	err := os.MkdirAll(fp.Dir(dest), 0750)
	errExit(err, "can't create dest dir: "+fp.Dir(dest))

	bb := "/home/xx/tools/busybox"
	c := bb + " grep \t" + file + " " + src + " > " + dest

	cmd := exec.Command(bb, "sh", "-c", c)
	out, err := cmd.CombinedOutput()

	errExit(err, "can't copy sha lines from "+src+" to "+dest+
		"\n"+string(out)+"\n"+strings.Join(cmd.Args, " "))

	c = bb + " sed -i '\\|\t" + file + "|d' " + src

	cmd = exec.Command(bb, "sh", "-c", c)
	out, err = cmd.CombinedOutput()

	errExit(err, "can't remove sha lines from "+src+
		"\n"+string(out)+"\n"+strings.Join(cmd.Args, " "))
}

func getSrc(genC genCfgT, pkg pkgT, pkgC pkgCfgT) {
	switch pkgC.src.srcType {
	case "tar":
		getSrcTar(pkg, pkgC)
	case "git":
		getSrcGitMaster(pkg, pkgC)
	case "go-mod":
		getViaGoMod(pkg, pkgC)
	case "alpine":
		getAlpinePkgs(genC, pkg, pkgC)
	}
}

func getSrcTar(pkg pkgT, pkgC pkgCfgT) {
	urls := strings.Split(pkgC.src.url, " ")

	for _, url := range urls {
		var fName string
		if strings.Contains(url, "::") {
			split := strings.Split(url, "::")
			url = split[1]
			fName = split[0]
		} else {
			fName = path.Base(url)
		}
		fPath := fp.Join(pkg.progDir, "src", fName)
		if fileExists(fPath) {
			return
		}

		fmt.Printf("  downloading %s...\n", fName)
		downloadToFile(url, fPath)
	}
}

func downloadToFile(url, fPath string) {
	client := &http.Client{
		CheckRedirect: func(req *http.Request,
			via []*http.Request) error {

			setHeaders(req)
			return nil
		},
	}
	req, err := http.NewRequest("GET", url, nil)
	errExit(err, "couldn't prepare a request for:\n  "+url)

	setHeaders(req)

	resp, err := client.Do(req)

	if err != nil || resp.StatusCode != 200 {
		fmt.Println("  download failed, getting tarball from gentoo...")

		url = "https://distfiles.gentoo.org/distfiles/" + path.Base(url)
		req, err = http.NewRequest("GET", url, nil)
		errExit(err, "couldn't prepare a request for "+url)

		setHeaders(req)

		resp, err = client.Do(req)
		errExit(err, "couldn't download src file from: "+url)
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			errExit(errors.New("non-200 response code"), url)
		}
	}

	fd, err := os.Create(fPath)
	errExit(err, "couldn't create src file")
	defer fd.Close()
	_, err = io.Copy(fd, resp.Body)
	errExit(err, "couldn't create src file")

	resp.Body.Close()

	if fileIsText(fPath) {
		errExit(errors.New(""),
			"downloaded tar is a text file:\n  "+fPath)
	}
}

func fileIsText(file string) bool {
	fd, err := os.Open(file)
	errExit(err, "can't open file:\n  "+file)
	defer fd.Close()

	buff := make([]byte, 8192)
	_, err = fd.Read(buff)
	errExit(err, "can't read file:\n  "+file)

	for _, b := range buff {
		if !unicode.IsPrint(rune(b)) {
			return false
		}
	}

	return utf8.Valid(buff)
}

func setHeaders(req *http.Request) {
	for k := range req.Header {
		delete(req.Header, k)
	}
	req.Header.Set("User-Agent", "Wget/1.2.1")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Connection", "Keep-Alive")
}

func getSrcGitMaster(pkg pkgT, pkgC pkgCfgT) {
	cloneDir := fp.Join(pkg.progDir, "src", pkg.prog)
	c := "git clone " + pkgC.src.url + " " + cloneDir

	if !fileExists(cloneDir) {
		fmt.Println("  git clone...")
	} else if !gitCommitExists(cloneDir, pkg.verShort) {
		fmt.Println("  git pull...")
		c = "cd " + cloneDir + " && git pull"
	} else {
		return
	}

	cmd := exec.Command("/home/xx/tools/busybox", "sh", "-c", c)
	err := cmd.Run()
	errExit(err, "can't get source from git server: "+pkgC.src.url)
}

func gitCommitExists(gitRoot, commit string) bool {
	c := "cd " + gitRoot + " && git rev-parse --quiet --verify " + commit
	cmd := exec.Command("/home/xx/tools/busybox", "sh", "-c", c)
	err := cmd.Run()

	if err != nil {
		return false
	}

	return true
}

func getViaGoMod(pkg pkgT, pkgC pkgCfgT) {
	srcDir := fp.Join(pkg.progDir, "src", pkg.ver)
	if fileExists(srcDir) {
		return
	}

	var uri string
	// todo: find a better way to determine golib version type
	if strings.Contains(pkg.ver, "-") {
		uri = pkgC.src.url + "@v0.0.0-" + pkg.ver
	} else {
		uri = pkgC.src.url + "@v" + pkg.ver
	}

	c := "go mod download " + uri
	cmd := exec.Command("/home/xx/tools/busybox", "sh", "-c", c)
	cmd.Env = []string{"GOPATH=" + srcDir,
		"GOCACHE=/tmp/xx/gocache",
		"PATH=/bin:/sbin:/usr/bin:/usr/sbin"}
	out, err := cmd.CombinedOutput()
	errExit(err, "can't get source with:\n  'go mod download "+uri+"'\n"+string(out))
}

func getAlpinePkgs(genC genCfgT, pkg pkgT, pkgC pkgCfgT) {
	repos := getAlpineRepos(genC, pkg, pkgC)
	rootFile := getAlpineRoot(pkg)
	createApkRoot(rootFile, pkgC.steps.buildDir, repos)

	instLxcConfig(genC, pkg, pkgC)

	pkgMap := makeAlpinePkgMap(pkg, pkgC, repos)
	for repoName, url := range pkgMap {
		repo := strings.Split(repoName, "/")[0]
		repoDir := fp.Dir(repos[repo])

		fName := path.Base(url)
		fPath := fp.Join(pkg.progDir, "src", fName)
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
		repoSplit := strings.Split(repoFile, "/")
		repo := strings.Join(repoSplit[:len(repoSplit)-2], "/")
		c += repo + "\n"
	}
	c += "'> etc/apk/repositories"

	cmd := exec.Command("/home/xx/tools/busybox", "sh", "-c", c)
	out, err := cmd.CombinedOutput()
	errExit(err, "can't create apk root: "+buildDir+"\n"+string(out))
}

func getAlpineRoot(pkg pkgT) string {
	url := "http://dl-cdn.alpinelinux.org/alpine/edge/releases/x86_64/alpine-minirootfs-20220328-x86_64.tar.gz"

	fName := path.Base(url)
	fPath := fp.Join(pkg.progDir, "src", fName)
	if !fileExists(fPath) {
		fmt.Printf("  downloading %s...\n", fName)
		downloadToFile(url, fPath)
	}

	return fPath
}

func getAlpineRepos(genC genCfgT, pkg pkgT, pkgC pkgCfgT) map[string]string {
	urls := strings.Split(pkgC.src.url, " ")
	repos := make(map[string]string)

	for i, url := range urls {
		urlSplit := strings.Split(url, "/")
		if len(urlSplit) < 3 {
			errExit(errors.New(""), "incorrect repo url")
		}

		repoName := urlSplit[len(urlSplit)-3]
		repoDir := fp.Join(pkg.progDir, "src/repo_"+genC.date,
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

func makeAlpinePkgMap(pkg pkgT, pkgC pkgCfgT, repos map[string]string) map[string]string {
	fmt.Printf("  making deps list...\n")
	apkDot := getAlpinePkgVer(pkg.prog)
	apkDot += " " + runApkDot(pkg.prog)
	for _, s := range pkgC.steps.env {
		split := strings.Split(s, "=")
		envVar := split[0]
		if envVar != "ADD_PROGS" {
			continue
		}

		addProgs := strings.Split(split[1], " ")
		for _, addProg := range addProgs {
			apkDot += " " + getAlpinePkgVer(addProg)
			apkDot += " " + runApkDot(addProg)
		}
	}
	split := strings.Split(strings.Replace(apkDot, "\"", "", -1), " ")

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

	cmd := exec.Command("/home/xx/tools/busybox", "sh", "-c", c)
	out, err := cmd.CombinedOutput()
	errExit(err, "can't get deps via apk: \n"+string(out))

	return string(out)
}

func getAlpinePkgVer(prog string) string {
	c := "lxc-execute -n xx -P /tmp/ -- sh -c \""
	c += "apk list " + prog + " | cut -d' ' -f1 \""

	cmd := exec.Command("/home/xx/tools/busybox", "sh", "-c", c)
	out, err := cmd.CombinedOutput()
	errExit(err, "can't get deps via apk: \n"+string(out))

	return strings.Replace(string(out), "\n", "", -1)
}

func getAlpinePkgRepo(name, ver string, repos map[string]string) string {
	for repo, repoFile := range repos {
		c := "tar xf " + repoFile + " APKINDEX -O | "
		c += "grep -q ^P:" + name + "$"
		cmd := exec.Command("/home/xx/tools/busybox", "sh", "-c", c)

		pkgNotExists := cmd.Run()
		if pkgNotExists == nil {
			return repo
		}
	}
	errExit(errors.New(""), "repo not found for pkg: "+name+"-"+ver)
	return ""
}

func getAlpinePkgNameVer(pkg string) (string, string) {
	split := strings.Split(pkg, "-")
	if len(split) < 3 {
		errExit(errors.New(""), "incorrect alpine pkg name: "+pkg)
	}
	name := strings.Join(split[:len(split)-2], "-")
	ver := strings.Join(split[len(split)-2:], "-")
	return name, ver
}

func makeBuildDirs(pkg pkgT, pkgC pkgCfgT) {
	dirs := []string{pkgC.tmpDir, pkgC.tmpLogDir}
	for _, d := range dirs {
		err := os.MkdirAll(d, 0700)
		errExit(err, "can't create tmp dir: "+d)
	}

	dirs = []string{"pkg", "src", "log"}
	for _, d := range dirs {
		dir := fp.Join(pkg.progDir, d)
		err := os.MkdirAll(dir, 0700)
		errExit(err, "can't create pkg dir: "+dir)
	}
}

func execStep(step string, genC genCfgT, pkg pkgT, pkgC pkgCfgT) {
	var command string
	pwd := pkgC.steps.buildDir

	pathOut := pkgC.tmpLogDir + "stdout-" + step + ".log"
	pathErr := pkgC.tmpLogDir + "stderr-" + step + ".log"

	switch step {
	case "prepare":
		fmt.Println("  preparing...")
		command = pkgC.steps.prepare
		pwd = pkgC.tmpDir
	case "configure":
		fmt.Println("  configuring...")
		command = pkgC.steps.configure
	case "build":
		fmt.Printf("  building %s...\n", pkg.setVerNewRel)
		command = pkgC.steps.build
	case "pkg_create":
		fmt.Println("  creating pkg...")
		command = pkgC.steps.pkg_create
	}

	if command == "" {
		return
	}

	fOut, err := os.Create(pathOut)
	errExit(err, "can't create log file")
	defer fOut.Close()

	fErr, err := os.Create(pathErr)
	errExit(err, "can't create log file")
	defer fErr.Close()

	instLxcConfig(genC, pkg, pkgC)
	cmd := prepareCmd(genC, pkg, pkgC, step, command, pwd, fOut, fErr)
	err = cmd.Run()
	if err != nil {
		// print error log when a step fails
		stderr := ""
		fd, _ := os.Open(pathErr)
		defer fd.Close()
		input := bufio.NewScanner(fd)
		for input.Scan() {
			stderr += input.Text() + "\n"
		}

		// meson for some reason prints errors to stdout
		if strings.Contains(pkgC.steps.configure, "meson") {
			fd, _ = os.Open(pathOut)
			defer fd.Close()
			print := false
			input = bufio.NewScanner(fd)
			for input.Scan() {
				line := strings.ToLower(input.Text())
				if strings.Contains(line, "error") ||
					strings.Contains(line, "failed") {
					print = true
				}
				if print {
					stderr += input.Text() + "\n"
				}
			}
		}

		// clean the new pkg dir on pkg_create error
		if step == "pkg_create" {
			remNewPkg(pkg, errors.New(""))
		}
		errExit(err, "can't execute command; stderr dump:\n\n"+stderr)
	}
}

func prepareCmd(genC genCfgT, pkg pkgT, pkgC pkgCfgT, step, command, pwd string, fOut, fErr *os.File) *exec.Cmd {

	shCmd := "lxc-execute"
	shCmdP := []string{"-n", "xx", "-P", "/tmp/xx/../"}
	for _, s := range pkgC.steps.env {
		envVar := strings.Split(s, "=")[0]
		shCmdP = append(shCmdP, "-s")
		shCmdP = append(shCmdP, "lxc.environment="+envVar)
	}
	shCmdP = append(shCmdP, []string{"--", "/home/xx/tools/ksh", "-c",
		"cd " + pwd + " && " + command}...)

	if pkgC.crossBuild {
		shCmd = "/bin/sh"
		shCmdP = []string{"-c", command}
	}

	if genC.rootDir == "/" {
		shCmd = "/home/xx/tools/ksh"
		shCmdP = []string{"-c", command}
	}

	cmd := exec.Command(shCmd, shCmdP...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = fOut
	cmd.Stderr = fErr
	cmd.Env = pkgC.steps.env
	cmd.Dir = pwd
	logCmd(pkg, pkgC, cmd, step)

	return cmd
}

func instLxcConfig(genC genCfgT, pkg pkgT, pkgC pkgCfgT) {
	var config string

	user, err := user.Current()
	errExit(err, "can't get user info")

	fd, err := os.Open("/home/xx/misc/lxc_config")
	errExit(err, "can't read lxc config file")
	input := bufio.NewScanner(fd)
	for input.Scan() {
		line := input.Text()
		isMapLine := strings.HasPrefix(line, "lxc.idmap =")
		if isMapLine && user.Username == "root" {
			continue
		}
		config += line + "\n"
	}
	fd.Close()

	replMap := setReplMap(genC, pkg, pkgC, pkgC.src, pkgC.steps)
	for k, v := range replMap {
		if k == "<root_dir>" && pkgC.src.srcType == "alpine" {
			v = pkgC.steps.buildDir + "/rootfs"
		}
		repl(&config, k, v)
	}
	repl(&config, "<user_id>", user.Uid)
	repl(&config, "<group_id>", user.Gid)

	dev := getMountDev(genC.rootDir)
	devBoot := getMountDev(genC.rootDir + "/boot")

	// add entries to pass devices from target dir mountpoints
	if dev != "" && devBoot != "" && genC.rootDir != "" {
		devCfg := "lxc.mount.entry = " + dev + " " +
			strings.Trim(dev, "/") +
			" none bind,create=file 0 0\n" +
			"lxc.mount.entry = " + devBoot + " " +
			strings.Trim(devBoot, "/") +
			" none bind,create=file 0 0\n"
		config += devCfg
	}

	if fileExists(genC.rootDir + "/mnt/xx/boot") {
		config += "lxc.mount.entry = /mnt/xx mnt/xx none bind 0 0\n"
		config += "lxc.mount.entry = /mnt/xx/boot mnt/xx/boot none bind 0 0"
	}

	err = os.MkdirAll("/tmp/xx/build", 0700)
	errExit(err, "couldn't create '/tmp/xx/' dir")

	fd, err = os.Create("/tmp/xx/config")
	errExit(err, "can't create lxc config file")
	defer fd.Close()

	_, err = io.Copy(fd, strings.NewReader(config))
	errExit(err, "can't write to lxc config file")
}

func getMountDev(mountPoint string) string {
	type fsT struct {
		Target  string `json:"target"`
		Source  string `json:"source"`
		Fstype  string `json:"fstype"`
		Options string `json:"options"`
	}
	type findMntT struct {
		Filesystems []fsT `json:"filesystems"`
	}

	cmd := exec.Command("/bin/findmnt", "-J", mountPoint)
	out, err := cmd.Output()
	if err == nil {
		var result findMntT
		err := json.Unmarshal(out, &result)
		if err != nil {
			os.Exit(1)
		}
		return result.Filesystems[0].Source
	}
	return ""
}

func remNewPkg(pkg pkgT, err error) {
	if err != nil {
		errRem := os.RemoveAll(pkg.newPkgDir)
		if errRem != nil {
			fmt.Fprintln(os.Stderr, "error: can't remove ", errRem)
		}
	}
}

func logCmd(pkg pkgT, pkgC pkgCfgT, cmd *exec.Cmd, step string) {
	path := fp.Join(pkgC.tmpLogDir, "cmd.log")

	fOpts := os.O_CREATE | os.O_APPEND | os.O_WRONLY
	fd, err := os.OpenFile(path, fOpts, 0644)
	errExit(err, "can't create cmd log file")
	defer fd.Close()

	cmdStr := fmt.Sprintf("%+v", cmd)
	cmdStr = strings.Replace(cmdStr, " -- ", " -- \n\n", -1)
	cmdStr = strings.Replace(cmdStr, "&& ", "&& \n\n", -1)

	fmt.Fprintf(fd, "[ %s ]\nENV: %+v\n\n%s\n\n\n",
		step, pkgC.steps.env, cmdStr)
}

func dumpSHA256(pkg pkgT) {
	files, err := walkDir(pkg.newPkgDir, "files")
	sort.Strings(files)
	remNewPkg(pkg, err)
	errExit(err, "can't get file list for: ")

	if len(files) == 0 {
		errExit(errors.New(""), "no files in pkg dir: "+pkg.newPkgDir)
	}

	var hashes string
	var sum string

	for _, file := range files {
		set, err := os.Stat(file)
		remNewPkg(pkg, err)
		errExit(err, "can't get file stat (broken link?): "+file)
		if set.IsDir() {
			continue
		}

		fd, err := os.Open(file)
		remNewPkg(pkg, err)
		errExit(err, "can't open file: "+file)

		hash := sha256.New()
		_, err = io.Copy(hash, fd)
		remNewPkg(pkg, err)
		errExit(err, "can't read file: "+file)
		fd.Close()

		sum = hex.EncodeToString(hash.Sum(nil))
		file = strings.TrimPrefix(file, pkg.newPkgDir)
		hashes += fmt.Sprintf("%s\t%s\n", sum, file)
	}

	pathOut := fp.Join(pkg.progDir, "log", pkg.setVerNewRel, "sha256.log")
	fOut, err := os.Create(pathOut)
	errExit(err, "can't create hash log file")
	defer fOut.Close()

	fmt.Fprintf(fOut, "%s", hashes)
}

// used only during build step
func dumpSharedLibs(world map[string]worldT, genC genCfgT, pkg pkgT) {
	files, err := walkDir(pkg.pkgDir, "files")

	sharedLibs := make(map[string]bool)
	for _, file := range files {
		fd, err := os.Open(file)
		errExit(err, "can't open "+file)

		elfBin, err := elf.NewFile(fd)
		if err != nil {
			fd.Close()
			continue
		}
		libs, err := elfBin.ImportedLibraries()
		errExit(err, "can't get imported libraries from "+file)
		fd.Close()

		for _, l := range libs {
			sharedLibs[l] = true
		}
	}

	if len(sharedLibs) == 0 {
		return
	}

	pathOut := fp.Join(pkg.progDir, "log", pkg.setVerRel, "shared_libs")
	fOut, err := os.Create(pathOut)
	errExit(err, "can't create shared libs file")
	defer fOut.Close()

	for lib := range sharedLibs {
		// exception for syslinux libraries
		if strings.HasSuffix(lib, ".c32") {
			continue
		}

		libPath := findLibPath(world, genC, lib)
		dep := world["/"].files[libPath]
		if libPath == "" {
			dep = pkg
		}
		fmt.Fprintf(fOut, "%s\t%s\t%s\t%s\t%s\n", lib, dep.name, dep.set, dep.ver, dep.rel)
	}
}

// used only during pkg build
func findLibPath(world map[string]worldT, genC genCfgT, lib string) string {
	ldSoConf := fp.Join(genC.rootDir, "/etc/ld.so.conf")
	if !fileExists(ldSoConf) {
		return ""
	}

	fd, err := os.Open(ldSoConf)
	errExit(err, "can't open ld.so.conf in "+ldSoConf)
	defer fd.Close()
	input := bufio.NewScanner(fd)

	for input.Scan() {
		ldLibraryPath := input.Text()
		libPath := fp.Join(ldLibraryPath, lib)
		_, found := world["/"].files[libPath]
		if found {
			return libPath
		}
	}

	return ""
}

func cleanup(pkg pkgT, pkgC pkgCfgT) {
	err := os.RemoveAll(pkgC.tmpDir)
	errExit(err, "can't remove tmp dir")

	pkgFiles, err := walkDir(pkg.newPkgDir, "files")
	errExit(err, "can't read pkg files")

	if !pkgC.crossBuild && !pkgC.muslBuild {
		rmStaticLibs(&pkgFiles)
	}
	stripDebug(&pkgFiles, pkg)

	rmEmptyLogs(pkg)
}

func moveLogs(pkg pkgT, pkgC pkgCfgT) {
	logDir := fp.Join(pkg.progDir, "log", pkg.setVerNewRel)
	cmd := exec.Command("/home/xx/tools/busybox", "cp", "-rd",
		pkgC.tmpLogDir, logDir)
	err := cmd.Run()
	errExit(err, "can't move log dir")
}

func saveHelp(genC genCfgT, pkg pkgT, pkgC pkgCfgT) {
	var c, helpType, file string
	switch {
	case fileExists(pkgC.steps.buildDir+"/configure") &&
		!strings.Contains(pkgC.steps.configure, "meson"):

		helpType = "command"
		c = "./configure --help ||:"

	case fileExists(pkgC.steps.buildDir + "/meson.build"):
		helpType = "command"
		c = "meson configure ||:"

	case fileExists(pkgC.steps.buildDir + "/CMakeLists.txt"):
		helpType = "command"
		c = "cd build && cmake -LAH . | grep -v " + pkgC.tmpDir + " ||:"

	case fileExists(pkgC.steps.buildDir + "/wscript"):
		helpType = "command"
		c = "/usr/bin/waf configure --help"

	// mostly for dnsmasq
	case fileExists(pkgC.steps.buildDir + "/src/config.h"):
		helpType = "file"
		file = pkgC.steps.buildDir + "/src/config.h"

	// mostly for st and dwm
	case fileExists(pkgC.steps.buildDir + "/config.def.h"):
		helpType = "file"
		file = pkgC.steps.buildDir + "/config.def.h"

	// wpa_supplicant
	case fileExists(pkgC.steps.buildDir + "/wpa_supplicant/defconfig"):
		helpType = "file"
		file = pkgC.steps.buildDir + "/wpa_supplicant/defconfig"

	// hostapd
	case fileExists(pkgC.steps.buildDir + "/hostapd/defconfig"):
		helpType = "file"
		file = pkgC.steps.buildDir + "/hostapd/defconfig"

	default:
		return
	}

	pathOut := fp.Join(pkg.progDir, "log", pkg.setVerNewRel,
		"config-help.log")

	switch helpType {
	case "command":
		fOut, err := os.Create(pathOut)
		errExit(err, "can't create config help file")
		defer fOut.Close()

		cmd := prepareCmd(genC, pkg, pkgC, "save_help", c,
			pkgC.steps.buildDir, fOut, fOut)
		err = cmd.Run()
		errExit(err, "can't execute config help")

	case "file":
		cmd := exec.Command("/home/xx/tools/busybox", "cp", file,
			pathOut)
		err := cmd.Run()
		errExit(err, "can't copy config-help file")
	}
}

func rmStaticLibs(pkgFiles *[]string) {
	for _, file := range *pkgFiles {
		if strings.HasSuffix(file, ".la") {
			err := os.Remove(file)
			errExit(err, "can't remove "+file)
		}
	}
}

func rmEmptyLogs(pkg pkgT) {
	logFiles, err := walkDir(fp.Join(pkg.progDir, "log", pkg.setVerNewRel),
		"files")
	errExit(err, "can't read log files")
	for _, file := range logFiles {
		info, err := os.Stat(file)
		errExit(err, "can't read "+file)
		if info.Size() == 0 {
			err := os.Remove(file)
			errExit(err, "can't remove "+file)
		}
	}
}

func stripDebug(pkgFiles *[]string, pkg pkgT) {
	for _, file := range *pkgFiles {
		var lib, usrLib, bin bool
		ext := fp.Ext(file)

		// do not touch go packages, these are not static libraries
		if strings.Contains(file, "/go/pkg/") && strings.HasSuffix(file, ".a") {
			continue
		}

		if strings.HasPrefix(file, pkg.newPkgDir+"/lib/") {
			lib = true
		} else if strings.HasPrefix(file, pkg.newPkgDir+"/usr/lib/") {
			usrLib = true
		}

		binDirs := []string{"/bin/", "/sbin/", "/usr/bin/",
			"/usr/sbin/", "/usr/libexec/", "/tools/bin"}
		for _, dir := range binDirs {
			if strings.HasPrefix(file, pkg.newPkgDir+dir) {
				bin = true
				break
			}
		}

		if lib && ext == ".a" {
			runStrip("--strip-debug", file)
		} else if (usrLib || lib) && strings.HasPrefix(ext, ".so") {
			runStrip("--strip-unneeded", file)
		} else if bin {
			// pie executables can't be stripped with --strip-all
			// relocation data is needed
			runStrip("--strip-unneeded", file)
		}
	}
}

func runStrip(arg, file string) {
	cmd := exec.Command("strip", arg, file)
	_, _ = cmd.Output()
}

func pressAKey() {
	fmt.Println("\n  new package dir is now going to be removed. " +
		"press any key to continue...")
	var b []byte = make([]byte, 1)
	os.Stdin.Read(b)
}
