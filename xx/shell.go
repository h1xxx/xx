package main

import (
	"fmt"
	"os"
	"os/exec"

	fp "path/filepath"
)

func (r *runT) actionShell() {
	r.createBuildDirs()

	// base env must exist first, copy it if it's not in place
	if !r.baseOk {
		prHigh("copying base dir...")
		r.installBase()
	}

	// create links in root dir to base dir
	if !r.baseLinked && !r.baseEnv && !r.isSepSys {
		linkBaseDir(r.rootDir, r.baseDir)
	}

	prHigh("using %s build environment...", r.buildEnv)
	r.createRootDirs()

	// if base environment is built protect it from further changes
	if r.baseEnv {
		_, _ = os.Create(fp.Join(r.baseDir, "base_ok"))
		protectBaseDir(r.baseDir)
	}

	for i, p := range r.pkgs {
		pc := r.pkgCfgs[i]

		if i == len(r.pkgs)-1 || p.name == r.shellPkgName {
			if r.shellExtract {
				fmt.Printf("+ %-32s %s\n", p.name, p.setVerRel)
				createPkgDirs(p, pc)
				os.Remove(p.newPkgDir)
				r.getSrc(p, pc)
				r.execStep("prepare", p, pc)
				r.execStep("configure", p, pc)
			}

			r.instLxcConfig(p, pc)
			startShell()
			break
		}

		if r.shellInstall {
			fmt.Printf("+ %-32s %s\n", p.name, p.setVerRel)
			r.instPkg(p, pc, "/")
			for _, s := range pc.steps.subPkgs {
				subPkg := getSubPkg(p, s.suffix)
				r.instPkg(subPkg, pc, "/")
			}
		}
	}
}

func startShell() {
	cmd := exec.Command("lxc-execute",
		"-n", "xx", "-P", "/tmp", "--", "/bin/bash", "-l")
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	cmd.Run()
}

func preparePkgForShell(p pkgT) {
}
