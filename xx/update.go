package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type repolPkgT struct {
	Name        string
	Repo        string `json:"Repo"`
	Srcname     string `json:"srcname"`
	Binname     string `json:"binname"`
	Visiblename string `json:"visiblename"`
	Version     string `json:"version"`
	Origversion string `json:"origversion"`
	Status      string `json:"status"`
	Summary     string `json:"summary"`
	Categories  string `json:"categories"`
	Licenses    string `json:"licenses"`
	Maintainers string `json:"maintainers"`
}

func actionUpdate(pkgs []pkgT, pkgCfgs []pkgCfgT) {
	showUpdateInfo(pkgs, pkgCfgs)
}

func showUpdateInfo(pkgs []pkgT, pkgCfgs []pkgCfgT) {
	for i, pkg := range pkgs {
		pkgC := pkgCfgs[i]
		repolPkgs := getRepolInfo(pkg)
		latestVer := getLatestVer(repolPkgs)
		if pkgC.src.srcType != "tar" {
			continue
		}
		fmtStr := "%-30s %15s %15s\n"
		if pkg.ver < latestVer {
			latestVer = "\033[1m" + latestVer + "\033[0m"
			fmtStr = "%-30s %15s %23s\n"
		}
		fmt.Printf(fmtStr, pkg.name, pkg.ver, latestVer)
	}
}

func getRepolInfo(pkg pkgT) []repolPkgT {

	prog := strings.Split(pkg.name, "/")[1]
	url := "https://repology.org/api/v1/project/" + prog
	resp, err := http.Get(url)
	errExit(err, "can't get info from repol on pkg:\n  "+url)
	defer resp.Body.Close()

	var repolPkgs []repolPkgT
	json.NewDecoder(resp.Body).Decode(&repolPkgs)

	for i := range repolPkgs {
		(&repolPkgs[i]).Name = pkg.name
	}

	return repolPkgs
}

func getLatestVer(repolPkgs []repolPkgT) string {
	var ver string
	for _, repolPkg := range repolPkgs {
		status := "newest"
		// exception for lts linux kernel
		if repolPkg.Name == "sys-kernel/linux" {
			status = "legacy"
			if repolPkg.Srcname != "linux-lts" {
				continue
			}
		}

		if repolPkg.Status == status && repolPkg.Version > ver {
			ver = repolPkg.Version
		}
	}
	return ver
}
