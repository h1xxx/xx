package main

import (
	"fmt"
	"os"
	fp "path/filepath"
)

func (r *runT) actionSource(pkgs []pkgT, pkgCfgs []pkgCfgT) {
	r.getAllSrc(pkgs, pkgCfgs)
}

func (r *runT) getAllSrc(pkgs []pkgT, pkgCfgs []pkgCfgT) {
	for i, pkg := range pkgs {
		pkgC := pkgCfgs[i]

		fmt.Printf("+ %-32s %s\n", pkg.name, pkg.ver)

		srcDir := fp.Join(pkg.progDir, "src")
		err := os.MkdirAll(srcDir, 0700)
		errExit(err, "couldn't create dir: "+srcDir)

		r.getSrc(pkg, pkgC)
	}
}
