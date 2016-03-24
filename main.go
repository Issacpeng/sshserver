package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"sync"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
)

var sshkeyLoadLocker = sync.Mutex{}

const (
	sshPortEnv     = "SSH_PORT"
	defaultSshPort = "22"
	serverIP       = "0.0.0.0"
	keyPath        = "ssh/id_rsa"
	RepoRootPath   = "myrepo"
)

type  Mode int

const (
	MODE_READ = iota  
	MODE_WRITE        
)

var (
	validateCommands = map[string]Mode{
		"git-upload-pack":    MODE_READ,
		"git-upload-archive": MODE_READ,
		"git-receive-pack":   MODE_WRITE,
	}
)

func getlistenaddress() (string, string) {
	port := os.Getenv(sshPortEnv)
	if port == "" {
		port = defaultSshPort
	} else { 
		port = fmt.Sprintf("%s", port)
	}

    listenaddress := fmt.Sprintf("%s:%s", serverIP, port)
    return listenaddress, port
}

func createKeygroup(keyPath string) {
	if _, err := os.Stat(keyPath); err != nil {
		os.MkdirAll(filepath.Dir(keyPath), os.ModePerm)
	    if err := exec.Command("ssh-keygen", "-f", keyPath, "-t", "rsa", "-N", "").Run(); err != nil {
			panic(fmt.Sprintf("SSH: Fail to generate private key: %v\r\n", err))
		}
		fmt.Printf("SSH: New private key is generateed: %s\r\n", keyPath)
	}
}

func createRepopath(repoPath string) {
	if _, err := os.Stat(repoPath); err != nil {
		if err := os.MkdirAll(repoPath, os.ModePerm); err != nil {
			panic(fmt.Sprintf("SSH: Fail to generate repo path: %v\r\n", err))
		}
		fmt.Printf("SSH: New repo path is generateed: %s\r\n", repoPath)
	}
}

func main() {
	config := &ssh.ServerConfig{
		NoClientAuth: true,
	}

	createKeygroup(keyPath)
	createRepopath(RepoRootPath)
    generateKey(keyPath, config)

    address, port := getlistenaddress()
	listener, err := net.Listen("tcp", address)
	if err != nil {
		panic("SSH: failed to listen for connection\r\n")
	}

	fmt.Printf("SSH: service listen at port: %v\r\n", port)
	for {
		nConn, err := listener.Accept()
		if err != nil {
			fmt.Printf("SSH: Error accepting incoming connection: %v\r\n", err)
			continue
		}
		go handleConn(nConn, config)
	}
}

func handleConn(conn net.Conn, config *ssh.ServerConfig) {
	fmt.Printf("SSH: Handshaking for %s\r\n", conn.RemoteAddr())
	sConn, chans, reqs, err := ssh.NewServerConn(conn, config)
	if err != nil {
		if err == io.EOF {
			fmt.Printf("SSH: Handshaking was terminated: %v\r\n", err)
		} else {
			fmt.Printf("SSH: Error on handshaking: %v\r\n", err)
		}
		return
	}

	fmt.Printf("SSH: Connection from %s (%s)\r\n", sConn.RemoteAddr(), sConn.ClientVersion())
	go ssh.DiscardRequests(reqs)

	for newChan := range chans {
		if newChan.ChannelType() != "session" {
			newChan.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}

		channel, requests, err := newChan.Accept()
		if err != nil {
			fmt.Println("SSH: could not accept channel.\r\n")
			return
		}
        
		go func(in <-chan *ssh.Request) {
			defer channel.Close()
			for req := range in {
				switch req.Type {
				case "exec":
					payload := string(req.Payload)
					i := strings.Index(payload, "git")
					if i == -1 {
						fmt.Sprintf("SSH: %s is invalidate, only support git command!\r\n", payload)
						continue
					}
	                cmd := payload[i:]           
                    handleGitcmd(cmd, req, channel)
					return
				}
			}
		}(requests)
	}
}

func handleGitcmd(cmd string, req *ssh.Request, channel ssh.Channel) {
    verb, args := parseGitcmd(cmd)
	_, has := validateCommands[verb]
	if !has {
		panic(fmt.Sprintf("SSH: Unknown git command %s\r\n", verb))
		return
	}

	gitcmd := generateGitcmd(verb, args)
	gitcmd.Dir = RepoRootPath

	gitcmdStart(gitcmd, req, channel)
	channel.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
}

func parseGitcmd(cmd string) (string, string) {
	cmdleft := strings.TrimLeft(cmd, "'()")
	cmdsplit := strings.SplitN(cmdleft, " ", 2)
	if len(cmdsplit) != 2 {
		return "", ""
	}
	return cmdsplit[0], strings.Replace(cmdsplit[1], "'/", "'", 1)
}

func generateGitcmd(verb string, args string) *exec.Cmd {
	repoPath := strings.ToLower(strings.Trim(args, "'"))
	verbs := strings.Split(verb, " ")

	var gitcmd *exec.Cmd
	if len(verbs) == 2 {
		gitcmd = exec.Command(verbs[0], verbs[1], repoPath)
	} else {
		gitcmd = exec.Command(verb, repoPath)
	}

	return gitcmd
}

func gitcmdStart(gitcmd *exec.Cmd, req *ssh.Request, channel ssh.Channel) {
	stdout, err := gitcmd.StdoutPipe()
	if err != nil {
		fmt.Fprintln(os.Stderr, "SSH: StdoutPipe: %v", err)
		return
	}
	stderr, err := gitcmd.StderrPipe()
	if err != nil {
		fmt.Fprintln(os.Stderr, "SSH: StderrPipe: %v", err)
		return
	}
	input, err := gitcmd.StdinPipe()
	if err != nil {
		fmt.Fprintln(os.Stderr, "SSH: StdinPipe: %v", err)
		return
	}

	if err = gitcmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "SSH: Start: %v", err)
		return
	}

	req.Reply(true, nil)
	go io.Copy(input, channel)
	io.Copy(channel, stdout)
	io.Copy(channel.Stderr(), stderr)

	if err = gitcmd.Wait(); err != nil {
		fmt.Fprintln(os.Stderr, "SSH: Wait: %v", err)
		return
	}
}

// generateKey add hostkey and pubkey
func generateKey(keyPath string, config *ssh.ServerConfig) {
	privateBytes, err := ioutil.ReadFile(keyPath)
	if err != nil {
		panic("SSH: Fail to load private key\r\n")
	}
	private, err := ssh.ParsePrivateKey(privateBytes)
	if err != nil {
		panic("SSH: Fail to parse private key\r\n")
	}
	config.AddHostKey(private)

    saveAuthorizedKeyFile(keyPath)
}

// saveAuthorizedKeyFile writes SSH pubkey content to authorized_keys file.
func saveAuthorizedKeyFile(keyPath string) error {
	sshkeyLoadLocker.Lock()
	defer sshkeyLoadLocker.Unlock()

	fpath := fmt.Sprintf("/%s/%s/%s", "root", ".ssh", "authorized_keys")
	_, err := os.Stat(fpath)
	if err != nil {
        if _, err := os.Create(fpath); err != nil {
        	panic(fmt.Sprintf("SSH: Fail to Create authorized_keys: %v\r\n", err))
        }
	    fmt.Printf("SSH: New authorized_keys is generated: %s\r\n", fpath)
	}

    pubkeypath := fmt.Sprintf("%s%s", keyPath, ".pub")
	if _, err := os.Stat(pubkeypath); err == nil {
		content, _ := ioutil.ReadFile(pubkeypath)
		if err := ioutil.WriteFile(fpath, content, 0700); err != nil {
			return err
		}
	}

	return nil
}