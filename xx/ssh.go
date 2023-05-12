package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/user"

	str "strings"

	ssh "golang.org/x/crypto/ssh"
	kh "golang.org/x/crypto/ssh/knownhosts"
)

type sshCfgT struct {
	host    string
	port    string
	address string
	privKey string
}

func sshConnExec(host, cmd string) {
	client := createSshClient(host)
	defer client.Close()

	session, err := client.NewSession()
	errExit(err, "failed to create ssh session")
	defer session.Close()

	stdOut, stdErr, err := runRemoteCmd(session, cmd)
	if err == nil {
		fmt.Println(stdOut)
	} else {
		fmt.Println(err, stdErr)
	}
}

func createSshClient(host string) *ssh.Client {
	homeDir, err := os.UserHomeDir()
	errExit(err, "can't get home dir")

	sshConfig := parseSshConfig(host, homeDir)

	key, err := ioutil.ReadFile(sshConfig.privKey)
	errExit(err, "unable to read private key")

	signer, err := ssh.ParsePrivateKey(key)
	errExit(err, "unable to parse private key")

	user, err := user.Current()
	errExit(err, "unable to get current user info")

	hostKeyCallback, err := kh.New(homeDir + "/.ssh/known_hosts")
	errExit(err, "unable to open or parse known_hosts")

	config := &ssh.ClientConfig{
		User: user.Username,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: hostKeyCallback,
	}

	hostPort := sshConfig.address + ":" + sshConfig.port
	client, err := ssh.Dial("tcp", hostPort, config)
	errExit(err, "unable to connect to remote host")

	return client
}

func parseSshConfig(host, homeDir string) sshCfgT {
	var sshConfig sshCfgT
	var hostFound bool
	sshConfig.host = host

	re := getRegexes()

	f, err := os.Open(homeDir + "/.ssh/config")
	errExit(err, "can't open ssh config")
	defer f.Close()

	input := bufio.NewScanner(f)
	for input.Scan() {
		line := input.Text()

		if line == "" || string(line[0]) == "#" {
			continue
		}

		line = re.wSpaces.ReplaceAllString(line, " ")
		line = str.Trim(line, " ")
		fields := str.Split(line, " ")
		key := str.ToLower(fields[0])

		if len(fields) == 1 {
			continue
		}

		switch {
		case hostFound && key == "hostname":
			sshConfig.address = fields[1]
			continue

		case hostFound && key == "port":
			sshConfig.port = fields[1]
			continue

		case hostFound && key == "identityfile":
			k := str.Replace(fields[1], "~", homeDir, -1)
			sshConfig.privKey = k
			continue

		case hostFound && key == "host":
			break
		}

		if key == "host" {
			for _, val := range fields[1:] {
				if val == host {
					hostFound = true
				}
			}
		}
	}

	if sshConfig.address == "" {
		errExit(nil, "unable to locate host address")
	}

	return sshConfig
}

func runRemoteCmd(session *ssh.Session, cmd string) (string, string, error) {
	var b bytes.Buffer
	var e bytes.Buffer
	session.Stdout = &b
	session.Stderr = &e
	err := session.Run(cmd)
	return b.String(), e.String(), err
}
