package main

import (
	"fmt"
	"os"

	"example.com/krn/internal/code"
	"example.com/krn/internal/context"
	"example.com/krn/internal/doctor"
	"example.com/krn/internal/eval"
	"example.com/krn/internal/exec"
	"example.com/krn/internal/find"
	"example.com/krn/internal/integrate"
	"example.com/krn/internal/repomap"
	"example.com/krn/internal/state"
	"example.com/krn/internal/verify"
	"example.com/krn/internal/workspace"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		return
	}
	var err error
	switch os.Args[1] {
	case "context":
		err = context.Run(os.Args[2:])
	case "find":
		err = find.Run(os.Args[2:])
	case "map":
		err = repomap.Run(os.Args[2:])
	case "code":
		err = code.Run(os.Args[2:])
	case "verify":
		err = verify.Run(os.Args[2:])
	case "state":
		err = state.Run(os.Args[2:])
	case "exec":
		err = exec.Run(os.Args[2:])
	case "eval":
		err = eval.Run(os.Args[2:])
	case "eval-suite":
		err = eval.RunSuite(os.Args[2:])
	case "eval-pi":
		err = eval.RunPi(os.Args[2:])
	case "eval-claude":
		err = eval.RunClaude(os.Args[2:])
	case "integrate":
		err = integrate.Run(os.Args[2:])
	case "doctor":
		err = doctor.Run()
	case "uninstall":
		err = integrate.Uninstall()
	case "version":
		fmt.Println(workspace.Version)
	default:
		usage()
		return
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "krn:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Println("krn " + workspace.Version + "\nusage: krn context|find|map|code|verify|state|exec|eval|eval-suite|eval-pi|eval-claude|integrate|doctor|uninstall")
}
