// Command probe tests the sandbox from the inside. The test of the package builds it as a static binary in an image with no other files.
//
// With the argument net, it tries the network, the file system and the input, and writes one line for each: the name of the try, then ok or the error. With sleep it sleeps for one minute, with spam it writes 1 MiB, and with exit N it exits with the code N.
package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		os.Exit(2)
	}
	switch os.Args[1] {
	case "net":
		try("dial", func() error {
			c, err := net.DialTimeout("tcp", "1.1.1.1:443", 3*time.Second)
			if err == nil {
				_ = c.Close()
			}
			return err
		})
		try("udp", func() error {
			c, err := net.Dial("udp", "8.8.8.8:53")
			if err != nil {
				return err
			}
			defer func() { _ = c.Close() }()
			_, err = c.Write([]byte{0})
			return err
		})
		try("resolve", func() error {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_, err := net.DefaultResolver.LookupHost(ctx, "example.com")
			return err
		})
		try("write-root", func() error { return os.WriteFile("/probe", []byte("x"), 0o600) })
		try("write-work", func() error { return os.WriteFile("/work/probe", []byte("x"), 0o600) })
		try("write-scratch", func() error { return os.WriteFile("/scratch/probe", []byte("x"), 0o600) })
		try("read-work", func() error {
			b, err := os.ReadFile("/work/input.txt")
			if err == nil && strings.TrimSpace(string(b)) != "42" {
				err = fmt.Errorf("read %q", b)
			}
			return err
		})
		fmt.Printf("uid %d\n", os.Getuid())
	case "sleep":
		time.Sleep(time.Minute)
	case "spam":
		os.Stdout.Write(make([]byte, 1<<20))
	case "exit":
		n, _ := strconv.Atoi(os.Args[2])
		os.Exit(n)
	}
}

func try(name string, f func() error) {
	if err := f(); err != nil {
		fmt.Printf("%s failed\n", name)
		return
	}
	fmt.Printf("%s ok\n", name)
}
