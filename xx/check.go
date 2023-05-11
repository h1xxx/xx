package main

import (
	"fmt"
)

func (r *runT) actionCheck() {
	r.checkAllPkgs()
}

func (r *runT) checkAllPkgs() {
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
			for _, p := range pkgs {
				fmt.Println(" ", p.name)
			}
		}
		fmt.Println()
	} else {
		fmt.Println("* no unknown duplicated files.")
	}
}
