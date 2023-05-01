package main

import (
	"fmt"
	"os"

	fp "path/filepath"
	str "strings"
)

func (r *runT) writeCmd() {
	f := fp.Join(r.dirs.cnt, "cmd")

	fd, err := os.OpenFile(f, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0777)
	errExit(err)

	// todo: escape shell special chars
	cmdLine := "#!/bin/bash\nexec " + r.bin + " " + str.Join(r.args, " ")

	_, err = fmt.Fprintf(fd, cmdLine)
	errExit(err)
}
