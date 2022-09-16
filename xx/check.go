package main

import (
	"fmt"
)

func actionCheck(genC genCfgT) {
	checkAllPkgs(genC)
}

func checkAllPkgs(genC genCfgT) {
	return
}

func checkFileDupes(filePkg map[string][]pkgT) {
	dupes := make(map[string][]pkgT)

	for file := range filePkg {
		if len(filePkg[file]) > 1 {
			dupes[file] = append(dupes[file], filePkg[file]...)
		}
	}

	if len(dupes) > 0 {
		fmt.Println("* duplicated files:\n")
		for file, pkgs := range dupes {
			fmt.Println(" ", file)
			for _, pkg := range pkgs {
				fmt.Println(" ", pkg.name)
			}
		}
		fmt.Println()
	} else {
		fmt.Println("* no unknown duplicated files.")
	}
}
