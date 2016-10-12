package main

// #cgo LDFLAGS: -lcrypt
// #include <unistd.h>
// #include <crypt.h>
import "C"

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/reconquest/hierr-go"
)

func crypt(password, salt string) string {
	return C.GoString(C.crypt(C.CString(password), C.CString(salt)))
}

func getPassword(prompt string) (string, error) {
	var (
		sttyEchoDisable = exec.Command("stty", "-F", "/dev/tty", "-echo")
		sttyEchoEnable  = exec.Command("stty", "-F", "/dev/tty", "echo")
	)

	fmt.Print(prompt)

	err := sttyEchoDisable.Run()
	if err != nil {
		return "", hierr.Errorf(
			err, "%q", sttyEchoDisable.Args,
		)
	}

	defer func() {
		sttyEchoEnable.Run()
		fmt.Println()
	}()

	stdin := bufio.NewReader(os.Stdin)
	password, err := stdin.ReadString('\n')
	if err != nil {
		return "", err
	}

	password = strings.TrimRight(password, "\n")

	return password, nil
}
