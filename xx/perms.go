package main

import (
	"bufio"
	"os"
	"os/user"
	"sort"
	"strconv"
	"syscall"

	fp "path/filepath"
	str "strings"
)

func (r *runT) setSysPerm() {
	perms, owners := getPermOwner(r.rootDir)
	recPaths, fixedPaths := getFixedPaths(r.rootDir, perms, owners)
	rootDirs := getRootDirs(recPaths, r.rootDir)

	// get files and dirs only for root directories defined in /etc/perms
	var paths []string
	for _, dir := range rootDirs {
		if !fileExists(dir) {
			continue
		}
		walkedPaths, err := walkDir(dir, "all")
		errExit(err, "couldn't list all paths in:\n  "+r.rootDir)
		paths = append(paths, walkedPaths...)
	}
	paths = append(paths, fixedPaths...)
	sort.Strings(paths)
	paths = uniqueSlice(paths)

	currentUser, err := user.Current()
	errExit(err, "can't get current user uid")
	setOwner := currentUser.Uid == "0"
	//fmt.Println("zzzzzzz", perms)

	for _, fullPath := range paths {
		if isSymLink(fullPath) || fullPath == r.rootDir || fullPath == "." {
			continue
		}

		info, err := os.Stat(fullPath)
		errExit(err, "can't stat file\n:  "+fullPath)

		if setOwner {
			newUid, newGid := getOwner(fullPath, r.rootDir, owners)
			stat := info.Sys().(*syscall.Stat_t)
			uid := int(stat.Uid)
			gid := int(stat.Gid)

			if uid != newUid || gid != newGid {
				err = os.Chown(fullPath, newUid, newGid)
				errExit(err, "can't change owner:\n"+fullPath)
			}
		}

		newMode := getMode(fullPath, r.rootDir, perms)

		if info.Mode() != newMode && newMode != 0 {
			err := os.Chmod(fullPath, newMode)
			errExit(err, "can't change mode:\n"+fullPath)
		}
	}
}

// returns a list fixed paths from /etc/perms and a list of paths to use
// for recursive matching;
// paths returned are relative to root directory
func getFixedPaths(rootDir string, perms, owners map[string]string) ([]string, []string) {
	var recPaths, fixedPaths []string
	cntList := getCntList(fp.Join(rootDir, "/cnt/rootfs/"))

	for path := range perms {
		// path for container is prefixed with "fc:" or "dc:"
		pForCnt := string(path[1]) == "c"

		// path ends with a slash for recursive matching
		slashSuffix := str.HasSuffix(path, "/")

		// extract the path without any prefixes
		_, path, _ := str.Cut(path, ":")
		path = fp.Clean(path)

		switch {
		case pForCnt:
			path = str.TrimPrefix(path, rootDir)
			for _, cnt := range cntList {
				cntPath := fp.Join(rootDir, "/cnt/rootfs",
					cnt, path)
				if slashSuffix {
					recPaths = append(recPaths, cntPath)
				} else {
					fixedPaths = append(fixedPaths, cntPath)
				}
			}

		case slashSuffix:
			recPaths = append(recPaths, path)

		default:
			fixedPaths = append(fixedPaths, path)
		}
	}

	for path := range owners {
		// path for container is prefixed with "c:"
		pForCnt := str.HasPrefix(path, "c:")

		// path ends with a slash for recursive matching
		slashSuffix := str.HasSuffix(path, "/")

		// extract the path without any prefixes
		_, path, _ := str.Cut(path, ":")
		path = fp.Clean(path)

		switch {
		case pForCnt:
			path = str.TrimPrefix(path, rootDir)
			for _, cnt := range cntList {
				cntPath := fp.Join(rootDir, "/cnt/rootfs",
					cnt, path)
				if slashSuffix {
					recPaths = append(recPaths, cntPath)
				} else {
					fixedPaths = append(fixedPaths, cntPath)
				}
			}

		case slashSuffix:
			recPaths = append(recPaths, path)

		default:
			fixedPaths = append(fixedPaths, path)
		}
	}

	sort.Strings(recPaths)
	sort.Strings(fixedPaths)
	recPaths = uniqueSlice(recPaths)
	fixedPaths = uniqueSlice(fixedPaths)

	return recPaths, fixedPaths
}

func getRootDirs(recPaths []string, rootDir string) []string {
	var rootDirs []string
	rootDir = fp.Clean(rootDir)

	for _, dir := range recPaths {
		d := fp.Dir(dir)

		for d != rootDir {
			if stringExists(d, recPaths) {
				break
			}
			d = fp.Dir(d)
		}

		upDir := fp.Dir(d)
		pathExists := stringExists(d, rootDirs)
		pathExists = pathExists || stringExists(upDir, rootDirs)
		pathExists = pathExists || stringExists(dir, rootDirs)

		if d != rootDir && d != "." && !pathExists {
			rootDirs = append(rootDirs, d)
		} else if d == rootDir && dir != "." && !pathExists {
			rootDirs = append(rootDirs, dir)
		}
	}

	return rootDirs
}

// return maps of paths to permission/ownership info
func getPermOwner(rootDir string) (map[string]string, map[string]string) {
	perms := make(map[string]string)
	owners := make(map[string]string)

	// read the main perms file
	file := rootDir + "/etc/perms"
	readPermsFile(file, rootDir, perms, owners)

	if !fileExists(rootDir + "/etc/perms.d") {
		return perms, owners
	}

	// read files in perms.d
	permsPathList, err := os.ReadDir(rootDir + "/etc/perms.d")
	errExit(err, "can't read dir: "+rootDir+"/etc/perms.d")

	for _, permsPath := range permsPathList {
		name := permsPath.Name()
		targetPath := fp.Join(rootDir, "/etc/perms.d/", name)
		readPermsFile(targetPath, rootDir, perms, owners)
	}

	return perms, owners
}

func getMode(fullPath, rootDir string, permsMap map[string]string) os.FileMode {
	var pType, perms string
	var permExists bool

	pStat, err := os.Stat(fullPath)
	errExit(err, "can't get stat:\n  "+fullPath)

	switch {
	case !pStat.IsDir():
		pType = "f"
	case pStat.IsDir():
		pType = "d"
	}

	// find direct definitions of perms
	perms = permsMap[pType+":"+fullPath]

	// find recursive permissions (the ones with a trailing slash)
	path := fullPath
	if perms == "" {
		for path != rootDir {
			perms, permExists = permsMap[pType+":"+path+"/"]
			if permExists {
				break
			}
			path = fp.Dir(path)
		}
	}

	if perms == "" {
		return 0
	}

	mode64, err := strconv.ParseUint(perms[1:4], 8, 32)
	errExit(err, "cannot parse mode for file:\n"+fullPath)
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
		errExit(ERR, "incorrect mode:", perms)
	}

	return mode
}

func getOwner(fullPath, rootDir string, ownerMap map[string]string) (int, int) {
	var owner string
	var ownerExists bool

	// find direct definitions of owners
	owner = ownerMap[fullPath]

	// find recursive ownerships (the ones with a trailing slash)
	path := fullPath
	if owner == "" {
		for path != rootDir {
			owner, ownerExists = ownerMap[path+"/"]
			if ownerExists {
				break
			}
			path = fp.Dir(path)
		}
	}

	if owner == "" {
		return 0, 0
	}

	o := str.Split(owner, ":")
	uid := ugIdLookup(o[0], rootDir+"/etc/passwd")
	gid := ugIdLookup(o[1], rootDir+"/etc/group")

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
		passLine := str.Split(input.Text(), ":")
		if passLine[0] == name {
			ugId, err := strconv.ParseInt(passLine[2], 10, 64)
			errExit(err, "can't parse uid/gid for "+name)

			return int(ugId)
		}
	}

	errExit(err, "can't find uid/gid for "+name)
	return 0
}

func readPermsFile(file, rootDir string, perms, owners map[string]string) {
	f, err := os.Open(file)
	errExit(err, "can't open file: "+file)
	defer f.Close()

	re := getRegexes()
	cntList := getCntList(fp.Join(rootDir, "/cnt/rootfs/"))

	input := bufio.NewScanner(f)
	for input.Scan() {
		line := input.Text()
		parsePermLine(line, rootDir, perms, owners, cntList, re)
	}
}

func parsePermLine(line, rootDir string, perms, owners map[string]string,
	cntList []string, re reT) {

	line = str.Trim(line, " ")
	if line == "" || string(line[0]) == "#" {
		return
	}
	line = re.wSpaces.ReplaceAllString(line, "\t")
	fields := str.Split(line, "\t")
	if len(fields) != 2 {
		errExit(ERR, "incorrect permissions:", line)
	}
	definedMsg := "permission already defined:\n" + line

	perm := fields[0]
	path := fields[1]

	// paths for containers always have prefix "fc:", "dc:" or "c:"
	var pForCnt bool
	if str.Contains(path, "c:") {
		pForCnt = true
	}

	// slash suffix in the path marks directories for recursive processing
	var slashTrail string
	if str.HasSuffix(path, "/") {
		slashTrail = "/"
	}

	// create full and valid paths for all the entries in /etc/perms
	switch {

	// ownerhsip has always first elemnt containing ':"
	// path field can have 'c:' prefix denoting path for /cnt/rootfs
	case str.Contains(perm, ":"):
		path = str.TrimPrefix(path, "c:")

		switch {
		case pForCnt:
			for _, cnt := range cntList {
				cntPath := fp.Join(rootDir, "/cnt/rootfs",
					cnt, path)
				_, pathExists := owners[cntPath+slashTrail]
				if pathExists {
					errExit(ERR, definedMsg)
				}
				owners[cntPath+slashTrail] = perm
			}
		default:
			path = fp.Join(rootDir, path)
			_, pathExists := owners[path+slashTrail]
			if pathExists {
				errExit(ERR, definedMsg)
			}
			owners[path+slashTrail] = perm
		}

	// file permissions have only digits
	case strDigitsOnly(perm):
		pType, path, found := str.Cut(path, ":")
		if !found || (pType[0] != 'f' && pType[0] != 'd') {
			errExit(ERR, "incorrect permissions:", line)
		}

		// exclude "c" in "fc:" and "dc:"
		pType = string(pType[0])

		switch {
		case pForCnt:
			for _, cnt := range cntList {
				cntPath := pType + ":" + fp.Join(rootDir,
					"/cnt/rootfs", cnt, path)
				_, pathExists := perms[cntPath+slashTrail]
				if pathExists {
					errExit(ERR, definedMsg)
				}
				perms[cntPath+slashTrail] = perm
			}
		default:
			path = pType + ":" + fp.Join(rootDir, path)
			_, pathExists := perms[path+slashTrail]
			if pathExists {
				errExit(ERR, definedMsg)
			}
			perms[path+slashTrail] = perm
		}

	// no other case allowed
	default:
		errExit(ERR, "incorrect permissions: "+line)
	}
}
