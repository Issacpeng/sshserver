package main

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Unknwon/com"
	"golang.org/x/crypto/ssh"
)

const (
	NoClientAuth = iota
	PublicKeyAuth
)

const (
	sshPortEnv     = "SSH_PORT"
	defaultSshPort = "22"
	keyPath        = "ssh/my_rsa"
	RepoRootPath   = "myrepo"
)

func generatekey(keyPath string) {
	if !com.IsExist(keyPath) {
		os.MkdirAll(filepath.Dir(keyPath), os.ModePerm)
		_, stderr, err := com.ExecCmd("ssh-keygen", "-f", keyPath, "-t", "rsa", "-N", "")
		if err != nil {
			panic(fmt.Sprintf("SSH: Fail to generate private key: %v - %s\r\n", err, stderr))
		}
		fmt.Printf("SSH: New private key is generateed: %s\r\n", keyPath)
	}
}

func main() {
	port := os.Getenv(sshPortEnv)
	if port == "" {
		port = defaultSshPort
	} else {
		port = fmt.Sprintf("%s", port)
	}

	config := &ssh.ServerConfig{
		NoClientAuth: true,
	}

	generatekey(keyPath)

	privateBytes, err := ioutil.ReadFile(keyPath)
	if err != nil {
		panic("SSH: Fail to load private key\r\n")
	}
	private, err := ssh.ParsePrivateKey(privateBytes)
	if err != nil {
		panic("SSH: Fail to parse private key\r\n")
	}
	config.AddHostKey(private)

	dl := NewDealDemo()
	go dl.Run()

	listener, err := net.Listen("tcp", "0.0.0.0:"+port)
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
		go handlerConn(nConn, dl, config)
	}
}

func cleanCommand(cmd string) string {
	i := strings.Index(cmd, "git")
	if i == -1 {
		return cmd
	}
	return cmd[i:]
}

func handlerConn(conn net.Conn, dl *DealDemo, config *ssh.ServerConfig) {
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
				payload := cleanCommand(string(req.Payload))
				switch req.Type {
				case "env":
					args := strings.Split(strings.Replace(payload, "\x00", "", -1), "\v")
					if len(args) != 2 {
						fmt.Fprintln(os.Stderr, "SSH: Invalid env arguments: '%#v'\r\n", args)
						continue
					}
					args[0] = strings.TrimLeft(args[0], "\x04")
					_, _, err := com.ExecCmdBytes("env", args[0]+"="+args[1])
					if err != nil {
						fmt.Fprintln(os.Stderr, "SSH: env: %v\r\n", err)
						return
					}
				case "exec":
					cmd := strings.TrimLeft(payload, "'()")
					gitcmd := genteatecmd(cmd)
					gitcmd.Dir = RepoRootPath

					fmt.Fprintln(os.Stderr, "Gogs RepoRootPath:", RepoRootPath)
					fmt.Fprintln(os.Stderr, "Gogs gitcmd.Dir:", gitcmd.Dir)
					fmt.Fprintln(os.Stderr, "Gogs gitcmd:", gitcmd)

					cmdstart(gitcmd, req, channel)
					channel.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
					return
				}
			}
		}(requests)
	}
}

func genteatecmd(cmd string) *exec.Cmd {
	verb, args := parseCmd(cmd)
	repoPath := strings.ToLower(strings.Trim(args, "'"))

	var gitcmd *exec.Cmd
	verbs := strings.Split(verb, " ")
	if len(verbs) == 2 {
		gitcmd = exec.Command(verbs[0], verbs[1], repoPath)
	} else {
		gitcmd = exec.Command(verb, repoPath)
	}

	return gitcmd
}

func cmdstart(gitcmd *exec.Cmd, req *ssh.Request, channel ssh.Channel) {
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

func parseCmd(cmd string) (string, string) {
	ss := strings.SplitN(cmd, " ", 2)
	if len(ss) != 2 {
		return "", ""
	}
	return ss[0], strings.Replace(ss[1], "'/", "'", 1)
}

type DealDemo struct {
	HandleChannel chan ssh.Channel
}

func NewDealDemo() *DealDemo {
	return &DealDemo{
		HandleChannel: make(chan ssh.Channel),
	}
}

func (dl *DealDemo) Run() {
	for {
		select {
		case c := <-dl.HandleChannel:
			go func() {
				reader := bufio.NewReader(c)
				fmt.Printf("SSH: reader:%v\r\n", reader)
			}()
		}
	}
}
