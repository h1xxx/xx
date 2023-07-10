package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"unicode"
	"unicode/utf8"

	fp "path/filepath"
	str "strings"
)

func (r *runT) actionSource() {
	r.getAllSrc(r.pkgs, r.pkgCfgs)
}

func (r *runT) getAllSrc(pkgs []pkgT, pkgCfgs []pkgCfgT) {
	for i, p := range pkgs {
		pc := pkgCfgs[i]

		fmt.Printf("+ %-32s %s\n", p.name, p.ver)

		srcDir := fp.Join(p.progDir, "src")
		err := os.MkdirAll(srcDir, 0700)
		errExit(err, "couldn't create dir: "+srcDir)

		r.getSrc(p, pc)
	}
}

func (r *runT) getSrc(p pkgT, pc pkgCfgT) {
	switch pc.src.srcType {
	case "tar":
		getSrcTar(p, pc)
	case "git":
		getSrcGitMaster(p, pc)
	case "go-mod":
		getViaGoMod(p, pc)
	case "alpine":
		r.getAlpinePkgs(p, pc)
	}
}

func getSrcTar(p pkgT, pc pkgCfgT) {
	urls := str.Split(pc.src.url, " ")

	for _, url := range urls {
		var fName string
		if str.Contains(url, "::") {
			split := str.Split(url, "::")
			url = split[1]
			fName = split[0]
		} else {
			fName = path.Base(url)
		}
		fPath := fp.Join(p.progDir, "src", fName)
		if fileExists(fPath) {
			return
		}

		fmt.Printf("  downloading %s...\n", fName)
		downloadToFile(p, url, fPath)
	}
}

func downloadToFile(p pkgT, url, fPath string) {
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
		fmt.Println("  download failed", url)
		fmt.Println("  getting tarball from gentoo...")

		url = "https://distfiles.gentoo.org/distfiles/" + path.Base(url)
		req, err = http.NewRequest("GET", url, nil)
		errExit(err, "couldn't prepare a request for "+url)

		setHeaders(req)

		resp, err = client.Do(req)
		errExit(err, "couldn't download src file from: "+url)
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			remNewPkg(p)
			errExit(ERR, "non-200 response code for:", url)
		}
	}

	fd, err := os.Create(fPath)
	errExit(err, "couldn't create src file")
	defer fd.Close()
	_, err = io.Copy(fd, resp.Body)
	errExit(err, "couldn't create src file")

	resp.Body.Close()

	if fileIsText(fPath) {
		errExit(ERR, "downloaded tarball is a text file:", fPath)
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

func getSrcGitMaster(p pkgT, pc pkgCfgT) {
	cloneDir := fp.Join(p.progDir, "src", p.prog)
	c := "git clone " + pc.src.url + " " + cloneDir

	if !fileExists(cloneDir) {
		fmt.Println("  git clone...")
	} else if !gitCommitExists(cloneDir, p.verShort) {
		fmt.Println("  git pull...")
		c = "cd " + cloneDir + " && git pull"
	} else {
		return
	}

	cmd := exec.Command("/home/xx/bin/busybox", "sh", "-c", c)
	err := cmd.Run()
	errExit(err, "can't get source from git server: "+pc.src.url)
}

func gitCommitExists(gitRoot, commit string) bool {
	c := "cd " + gitRoot + " && git rev-parse --quiet --verify " + commit
	cmd := exec.Command("/home/xx/bin/busybox", "sh", "-c", c)
	err := cmd.Run()

	if err != nil {
		return false
	}

	return true
}

func getViaGoMod(p pkgT, pc pkgCfgT) {
	srcDir := fp.Join(p.progDir, "src", p.ver)
	if fileExists(srcDir) {
		return
	}

	var uri string
	// todo: find a better way to determine golib version type
	if str.Contains(p.ver, "-") {
		uri = pc.src.url + "@v0.0.0-" + p.ver
	} else {
		uri = pc.src.url + "@v" + p.ver
	}

	c := "go mod download " + uri
	cmd := exec.Command("/home/xx/bin/busybox", "sh", "-c", c)
	cmd.Env = []string{"GOPATH=" + srcDir,
		"GOCACHE=/tmp/xx/gocache",
		"PATH=/bin:/sbin:/usr/bin:/usr/sbin"}
	out, err := cmd.CombinedOutput()
	errExit(err, "can't get source with:\n  'go mod download "+uri+"'\n"+string(out))
}
