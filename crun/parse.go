package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"

	fp "path/filepath"
	str "strings"
)

func (r *runT) parseArgs() {
	// indicates if args for crun (starting with '+') are still processed
	var cRunArgsDone, skipArg bool

	for i, arg := range os.Args {
		if skipArg {
			skipArg = false
			continue
		}

		if i == 0 {
			r.bin = fp.Base(arg)
			continue
		}

		if arg == "" {
			continue
		}

		if i == 1 && r.bin == "crun" && arg[0] != '-' {
			r.bin = fp.Base(arg)
			continue
		}

		if r.bin == "crun" && arg[0] == '-' {
			switch arg {
			case "-l", "--links":
				r.link = true
			case "-c", "--config":
				if len(os.Args)-1 >= i+1 {
					r.cntCfgFile = os.Args[i+1]
					skipArg = true
				}
			case "-D", "--debug":
				r.debug = true
			default:
				msg := "undefined arg: %s"
				errExit(fmt.Errorf(msg, arg))
			}
			continue
		}

		if !cRunArgsDone && arg[0] == '+' {
			switch arg {
			case "+c":
				if len(os.Args)-1 >= i+1 {
					r.cntCfgFile = os.Args[i+1]
					skipArg = true
				}
			case "+g":
				r.get = true
				if len(os.Args)-1 >= i+1 {
					absPath, err := fp.Abs(os.Args[i+1])
					errExit(err)
					r.getDest = absPath
					skipArg = true
				}
			case "+s":
				r.shell = true
			case "+n":
				r.writeCfg = false
			case "+b":
				if len(os.Args)-1 >= i+1 {
					absPath, err := fp.Abs(os.Args[i+1])
					errExit(err)
					r.bindWork = append(r.bindWork, absPath)
					skipArg = true
				}
			case "+d":
				r.debug = true
			default:
				msg := "undefined arg in crun: %s"
				errExit(fmt.Errorf(msg, arg))
			}
			continue
		}

		if arg[0] != '+' {
			cRunArgsDone = true
		}

		if str.ContainsAny(arg, "\n\t\\") {
			formatS := "arg can't contain \\n, \\t and \\: %s"
			errExit(fmt.Errorf(formatS, arg))
		}

		if fileExists(arg) {
			absPath, err := fp.Abs(arg)
			errExit(err)

			bindPath := fp.Join("/bind", absPath)

			r.args = append(r.args, quoteArg(bindPath))
			r.bindTargets = append(r.bindTargets, absPath)
		} else {
			quotedArg := quoteArg(arg)
			r.args = append(r.args, quotedArg)
		}

		if shouldHaveNet(arg) {
			r.cntConf.net = true
		}
	}
}

func quoteArg(arg string) string {
	switch {
	case str.Contains(arg, "'") && str.Contains(arg, "\""):
		errExit(fmt.Errorf("arg can't have ' and \" chars: %s", arg))
	case str.Contains(arg, "'"):
		return "\"" + arg + "\""
	case str.Contains(arg, "\""):
		return "'" + arg + "'"
	}

	return "'" + arg + "'"
}

func (r *runT) parseConf() {
	fd, err := os.Open(r.cntCfgFile)
	errExit(err)
	defer fd.Close()

	type sectionT struct {
		gen, bins, conf bool
	}

	r.cntId = 1
	reWSpace := regexp.MustCompile(`\s+`)

	var s sectionT
	var prevCnt string

	input := bufio.NewScanner(fd)
	for input.Scan() {
		line := str.TrimSpace(input.Text())

		if line == "" || line[0] == '#' {
			continue
		}

		switch line {
		case "[ general ]":
			s.gen, s.bins, s.conf = true, false, false
			continue
		case "[ container bins ]":
			s.gen, s.bins, s.conf = false, true, false
			continue
		case "[ container config ]":
			s.gen, s.bins, s.conf = false, false, true
			continue
		}

		switch {
		case s.gen:
			r.parseConfGen(line, reWSpace)
		case s.bins:
			cnt, bin := getKeyVal(line, reWSpace)
			if cnt != prevCnt {
				r.cntId++
			}
			if bin == r.bin {
				r.cnt = cnt
				s.bins = false
				continue
			}
			prevCnt = cnt
		case s.conf:
			found := r.parseConfCnt(line, reWSpace)
			if found {
				break
			}
		}
	}

	lastOctet := fmt.Sprintf(".%d/", r.cntId)
	r.cntIP = str.Replace(r.cntNetwork, ".0/", lastOctet, 1)
}

func (r *runT) parseConfGen(line string, reWSpace *regexp.Regexp) {
	key, val := getKeyVal(line, reWSpace)

	switch key {
	case "net":
		if !str.HasSuffix(val, ".0/24") {
			errExit(fmt.Errorf("no .0/24 suffix in network"))
		}
		r.cntNetwork = val
	default:
		errExit(fmt.Errorf("incorrect cnt.conf key: %s", key))
	}
}

func (r *runT) parseConfCnt(line string, reWSpace *regexp.Regexp) bool {
	cnt, conf := getKeyVal(line, reWSpace)

	if cnt != r.cnt {
		return false
	}

	r.cntConfStr = conf
	fields := str.Split(conf, ",")

	for _, f := range fields {
		switch f {
		case "tty0":
			r.cntConf.tty0 = true
		case "tty4":
			r.cntConf.tty4 = true
		case "fb":
			r.cntConf.fb = true
		case "dri":
			r.cntConf.dri = true
		case "snd":
			r.cntConf.snd = true
		case "input":
			r.cntConf.input = true
		case "net":
			r.cntConf.net = true
		case "udev":
			r.cntConf.udev = true
		case "sdl":
			r.cntConf.sdl = true
		case "shared":
			r.cntConf.shared = true
		default:
			errExit(fmt.Errorf("incorrect cnt.conf value: %s", f))
		}
	}

	return true
}

func getKeyVal(line string, reWSpace *regexp.Regexp) (string, string) {
	line = reWSpace.ReplaceAllString(line, " ")
	line = str.Replace(line, " = ", "=", 1)

	key, val, found := str.Cut(str.Replace(line, " ", "", -1), "=")
	if !found {
		errExit(fmt.Errorf("incorrect cnt.conf line: %s", line))
	}

	return key, val
}

func shouldHaveNet(s string) bool {
	switch {
	case str.Contains(s, "://"):
		return true
	case str.Contains(s, "youtube.com"):
		return true
	case str.Contains(s, "invidious"):
		return true
	}
	return false
}
