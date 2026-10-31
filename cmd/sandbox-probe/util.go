//go:build linux

package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

func holdForever() {
	for {
		time.Sleep(time.Hour)
	}
}

// errnoName names the errno inside err, or "" if there is none.
func errnoName(err error) string {
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return ""
	}
	if name, ok := errnoNames[errno]; ok {
		return name
	}
	return fmt.Sprintf("errno%d", int(errno))
}

var errnoNames = map[syscall.Errno]string{
	syscall.EPERM: "EPERM", syscall.ENOENT: "ENOENT", syscall.EACCES: "EACCES", syscall.EROFS: "EROFS",
	syscall.EAGAIN: "EAGAIN", syscall.ENOSPC: "ENOSPC", syscall.ENOMEM: "ENOMEM", syscall.EINVAL: "EINVAL",
	syscall.ENETUNREACH: "ENETUNREACH", syscall.ECONNREFUSED: "ECONNREFUSED", syscall.EHOSTUNREACH: "EHOSTUNREACH",
	syscall.EEXIST: "EEXIST", syscall.ENOTDIR: "ENOTDIR",
}

func runPlant(args []string) int {
	if len(args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: sandbox-probe plant DIR TEXT")
		return 2
	}
	if err := os.MkdirAll(args[0], 0o700); err != nil {
		fmt.Fprintln(os.Stderr, "plant:", errnoName(err))
		return 1
	}
	if err := os.WriteFile(filepath.Join(args[0], "secret"), []byte(args[1]), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "plant:", errnoName(err))
		return 1
	}
	fmt.Println("planted")
	holdForever()
	return 0
}

// runRead prints the size and digest of a file. The content itself never
// leaves the process, so a leak cannot show up in test output.
func runRead(args []string) int {
	if len(args) != 1 {
		return 2
	}
	data, err := os.ReadFile(args[0])
	if err != nil {
		fmt.Println("error", errnoName(err))
		return 1
	}
	sum := sha256.Sum256(data)
	fmt.Println("read", len(data), hex.EncodeToString(sum[:]))
	return 0
}

func runListen(args []string) int {
	if len(args) != 2 {
		return 2
	}
	ln, err := net.Listen("tcp", args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "listen:", err)
		return 1
	}
	fmt.Println("listening", ln.Addr())
	for {
		conn, err := ln.Accept()
		if err != nil {
			return 1
		}
		_, _ = io.WriteString(conn, args[1]+"\n")
		conn.Close()
	}
}

func runDial(args []string) int {
	if len(args) != 1 {
		return 2
	}
	conn, err := net.DialTimeout("tcp", args[0], 3*time.Second)
	if err != nil {
		fmt.Println("error", errnoName(err))
		return 1
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	line, _ := bufio.NewReader(conn).ReadString('\n')
	fmt.Println("connected", strings.TrimSpace(line))
	return 0
}

func runResolve(args []string) int {
	if len(args) != 1 {
		return 2
	}
	addrs, err := net.LookupHost(args[0])
	if err != nil {
		fmt.Println("error")
		return 1
	}
	fmt.Println(strings.Join(addrs, " "))
	return 0
}

func memoryBomb() {
	const chunk = 64 << 20
	var held [][]byte
	for {
		block := make([]byte, chunk)
		for i := 0; i < len(block); i += 4096 {
			block[i] = 1
		}
		held = append(held, block)
	}
}

// runProcs prints the command name of every process, sorted, one per line.
func runProcs() int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 1
	}
	var names []string
	for _, e := range entries {
		if comm, err := os.ReadFile("/proc/" + e.Name() + "/comm"); err == nil && isPID(e.Name()) {
			names = append(names, strings.TrimSpace(string(comm)))
		}
	}
	sort.Strings(names)
	_ = json.NewEncoder(os.Stdout).Encode(names)
	return 0
}

func isPID(name string) bool {
	for _, r := range name {
		if r < '0' || r > '9' {
			return false
		}
	}
	return name != ""
}

func runLs(args []string) int {
	if len(args) != 1 {
		return 2
	}
	entries, err := os.ReadDir(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "ls:", errnoName(err))
		return 1
	}
	names := []string{}
	for _, e := range entries {
		names = append(names, e.Name())
	}
	_ = json.NewEncoder(os.Stdout).Encode(names)
	return 0
}
