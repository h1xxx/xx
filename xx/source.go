package main

import (
	"fmt"
	"os"
	fp "path/filepath"
)

func actionSource(genC genCfgT, pkgs []pkgT, pkgCfgs []pkgCfgT) {
	getAllSrc(genC, pkgs, pkgCfgs)
}

func getAllSrc(genC genCfgT, pkgs []pkgT, pkgCfgs []pkgCfgT) {
	for i, pkg := range pkgs {
		pkgC := pkgCfgs[i]

		fmt.Printf("+ %-32s %s\n", pkg.name, pkg.ver)

		srcDir := fp.Join(pkg.progDir, "src")
		err := os.MkdirAll(srcDir, 0700)
		errExit(err, "couldn't create dir: "+srcDir)

		getSrc(genC, pkg, pkgC)
	}
}
