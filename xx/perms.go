package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/user"
	fp "path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

func setSysPerm(rootDir string) {
	// set permissions for root dir
	fmt.Println("  " + rootDir)
	setTargetPerm(rootDir, false)

	// set permissions for containers in /cnt/rootfs
	cntPathList, err := os.ReadDir(rootDir + "/cnt/rootfs")
	errExit(err, "can't read dir: "+rootDir+"/cnt/rootfs")

	for _, cntPath := range cntPathList {
		cntName := cntPath.Name()
		targetPath := fp.Join(rootDir, "/cnt/rootfs/", cntName)
		fmt.Println("  " + targetPath)
		if !fileExists(targetPath + "/etc/perms") {
			continue
		}
		setTargetPerm(targetPath, true)
	}
}

func setTargetPerm(targetDir string, cnt bool) {
	perms, owners := getPermOwner(targetDir)
	recDirs, definedFiles := getDefinedFiles(perms, owners, cnt)
	rootDirs := getRootDirs(recDirs, targetDir)

	var files []string
	for _, dir := range rootDirs {
		if !fileExists(dir) {
			continue
		}
		walkedFiles, err := walkDir(dir, "all")
		errExit(err, "couldn't list all files in:\n  "+targetDir)
		files = append(files, walkedFiles...)
	}
	files = append(files, definedFiles...)
	sort.Strings(files)
	files = uniqueSlice(files)

	currentUser, err := user.Current()
	errExit(err, "can't get current user uid")
	setOwner := currentUser.Uid == "0"

	for _, file := range files {
		if isSymLink(file) || file == targetDir {
			continue
		}

		// xxFile is a file in the xx system without the prefix dir
		// from the host system
		xxFile := file
		if targetDir != "/" {
			xxFile = strings.TrimPrefix(file, targetDir)
		}

		info, err := os.Stat(file)
		errExit(err, "can't stat file\n:  "+file)

		if setOwner {
			newUid, newGid := getOwner(xxFile, targetDir,
				owners, cnt)
			stat := info.Sys().(*syscall.Stat_t)
			uid := int(stat.Uid)
			gid := int(stat.Gid)

			if uid != newUid || gid != newGid {
				err = os.Chown(file, newUid, newGid)
				errExit(err, "can't change owner for:\n  "+file)
			}
		}

		newMode := getMode(xxFile, targetDir, perms, cnt)

		if info.Mode() != newMode && newMode != 0 {
			err := os.Chmod(file, newMode)
			errExit(err, "can't change mode for:\n  "+file)
		}
	}
}

func getDefinedFiles(perms, owners map[string]string, cnt bool) ([]string, []string) {
	var recDirs, definedFiles []string

	for path := range perms {
		pathSplit := strings.Split(path, ":")
		if !cnt && strings.Contains(pathSplit[0], "c") {
			continue
		}
		if strings.HasSuffix(pathSplit[1], "/") {
			recDirs = append(recDirs, pathSplit[1])
		} else {
			definedFiles = append(definedFiles, pathSplit[1])
		}
	}

	for path := range owners {
		pathSplit := strings.Split(path, ":")
		if !cnt && len(pathSplit) == 2 {
			continue
		}
		if len(pathSplit) == 2 {
			path = pathSplit[1]
		}
		if strings.HasSuffix(path, "/") {
			recDirs = append(recDirs, path)
		} else {
			definedFiles = append(definedFiles, path)
		}
	}

	sort.Strings(recDirs)
	sort.Strings(definedFiles)
	recDirs = uniqueSlice(recDirs)
	definedFiles = uniqueSlice(definedFiles)

	return recDirs, definedFiles
}

func getRootDirs(recDirs []string, targetDir string) []string {
	var rootDirs []string
	targetDir = fp.Clean(targetDir)

	for _, dir := range recDirs {
		d := fp.Dir(fp.Clean(dir))

		for d != targetDir {
			if stringExists(d+"/", recDirs) {
				break
			}
			d = fp.Dir(d)
		}

		upDir := fp.Dir(d)
		pathExists := stringExists(d+"/", rootDirs)
		pathExists = pathExists || stringExists(upDir+"/", rootDirs)
		pathExists = pathExists || stringExists(dir, rootDirs)

		if d != targetDir && !pathExists {
			rootDirs = append(rootDirs, d+"/")
		} else if d == targetDir && !pathExists {
			rootDirs = append(rootDirs, dir)
		}
	}

	return rootDirs
}

// return map of paths to permission/ownership info
func getPermOwner(targetDir string) (map[string]string, map[string]string) {
	perms := make(map[string]string)
	owners := make(map[string]string)

	// read the main perms file
	file := targetDir + "/etc/perms"
	readPermsFile(file, targetDir, perms, owners)

	if !fileExists(targetDir + "/etc/perms.d") {
		return perms, owners
	}

	// read files in perms.d
	permsPathList, err := os.ReadDir(targetDir + "/etc/perms.d")
	errExit(err, "can't read dir: "+targetDir+"/etc/perms.d")

	for _, permsPath := range permsPathList {
		name := permsPath.Name()
		targetPath := fp.Join(targetDir, "/etc/perms.d/", name)
		readPermsFile(targetPath, targetDir, perms, owners)
	}

	return perms, owners
}

func getMode(path, targetDir string, permsMap map[string]string, cnt bool) os.FileMode {
	var pType, perms string
	var permExists bool
	targetDir = fp.Clean(targetDir)

	p := fp.Join(targetDir, path)
	pStat, err := os.Stat(p)
	errExit(err, "can't get stat:\n  "+p)

	switch {
	case pStat.IsDir():
		pType = "d"
	case !pStat.IsDir():
		pType = "f"
	}

	if !cnt && strings.HasPrefix(path, "/cnt/rootfs") {
		pathLen := len(strings.Split(path, "/"))
		if pathLen >= 6 {
			return 0
		}
	}

	if cnt {
		pType += "c"
	}

	// find direct definitions of perms
	perms = permsMap[pType+":"+p]

	// find recursive permissions (the ones with a trailing slash)
	if perms == "" {
		for p != targetDir {
			perms, permExists = permsMap[pType+":"+p+"/"]
			if permExists {
				break
			}
			p = fp.Dir(p)
		}
	}

	if perms == "" {
		return 0
	}

	mode64, err := strconv.ParseUint(perms[1:4], 8, 32)
	errExit(err, "cannot parse mode for file\n  "+path)
	mode := os.FileMode(mode64)

	switch rune(perms[0]) {
	case '0':
		return mode
	case '1':
		return mode | os.ModeSticky
	case '2':
		return mode | os.ModeSetgid
	case '3':
		return mode | os.ModeSticky | os.ModeSetgid
	case '4':
		return mode | os.ModeSetuid
	case '5':
		return mode | os.ModeSticky | os.ModeSetuid
	case '6':
		return mode | os.ModeSetgid | os.ModeSetuid
	case '7':
		return mode | os.ModeSticky | os.ModeSetgid | os.ModeSetuid
	default:
		errExit(errors.New(""), "incorrect mode\n  "+perms)
	}

	return mode
}

func getOwner(path, targetDir string, ownerMap map[string]string, cnt bool) (int, int) {
	var owner, pType string
	var ownerExists bool
	path = fp.Clean(path)
	targetDir = fp.Clean(targetDir)

	p := fp.Join(targetDir, path)

	if !cnt && strings.HasPrefix(path, "/cnt/rootfs") {
		pathLen := len(strings.Split(path, "/"))
		if pathLen >= 6 {
			return 0, 0
		}
	}

	if cnt {
		pType = "c:"
	}

	// find direct definitions of owners
	owner = ownerMap[pType+p]

	// find recursive ownerships (the ones with a trailing slash)
	if owner == "" {
		for p != targetDir {
			owner, ownerExists = ownerMap[pType+p+"/"]
			if ownerExists {
				break
			}
			p = fp.Dir(p)
		}
	}

	if owner == "" {
		return 0, 0
	}
	o := strings.Split(owner, ":")

	uid := ugIdLookup(o[0], targetDir+"/etc/passwd")
	gid := ugIdLookup(o[1], targetDir+"/etc/group")

	return uid, gid
}

func ugIdLookup(name, lookupFile string) int {
	fd, err := os.Open(lookupFile)
	errExit(err, "can't open file: "+lookupFile)
	defer fd.Close()

	if name == "root" {
		return 0
	}

	input := bufio.NewScanner(fd)
	for input.Scan() {
		passLine := strings.Split(input.Text(), ":")
		if passLine[0] == name {
			ugId, err := strconv.ParseInt(passLine[2], 10, 64)
			errExit(err, "can't parse uid/gid for "+name)

			return int(ugId)
		}
	}

	errExit(err, "can't find uid/gid for "+name)
	return 0
}
