package sandbox_test

import (
	"fmt"
	"strings"

	"github.com/tamnd/math2-agent/sandbox"
)

func ExampleValidImage() {
	fmt.Println(sandbox.ValidImage("python:3"))
	fmt.Println(sandbox.ValidImage("m2-sage@sha256:" + strings.Repeat("ab", 32)))
	// Output:
	// false
	// true
}

func ExampleDocker_Args() {
	d := &sandbox.Docker{Runtime: "runsc"}
	args, err := d.Args(sandbox.Spec{
		Image:   "m2-sage@sha256:" + strings.Repeat("ab", 32),
		Command: []string{"/bin/sage", "/work/check.sage"},
		Dir:     "/home/me/math2/computations/586J26",
		Memory:  2 << 30,
	}, "m2-586J26")
	if err != nil {
		panic(err)
	}
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "--") && args[i] != "--" && args[i] != "--rm" && args[i] != "--read-only" {
			fmt.Println(args[i], args[i+1])
			i++
		}
	}
	// Output:
	// --name m2-586J26
	// --network none
	// --cap-drop ALL
	// --security-opt no-new-privileges
	// --pids-limit 256
	// --cpus 1
	// --memory 2147483648
	// --memory-swap 2147483648
	// --user 65534:65534
	// --tmpfs /scratch:rw,exec,size=268435456,mode=1777
	// --tmpfs /tmp:rw,size=268435456,mode=1777
	// --workdir /scratch
	// --env HOME=/scratch
	// --runtime runsc
	// --mount type=bind,source=/home/me/math2/computations/586J26,target=/work,readonly
}
