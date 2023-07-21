package main

import (
	"fmt"
	"os"
	"syscall"

	fp "path/filepath"
	str "strings"
)

func (r *runT) writeCmd() {
	f := fp.Join(r.dirs.cnt, "cmd")

	oldUmask := syscall.Umask(0)
	fd, err := os.OpenFile(f, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0777)
	syscall.Umask(oldUmask)
	errExit(err)
	defer fd.Close()

	// todo: escape shell special chars
	cmdLine := "#!/bin/bash\nexec " + r.bin + " " + str.Join(r.args, " ")

	_, err = fmt.Fprintf(fd, cmdLine)
	errExit(err)
}
