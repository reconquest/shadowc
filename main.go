package main

import (
	"fmt"
	"os"
	"os/user"
)

func init() {
	user, err := user.Current()
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	if user.Gid != "0" {
		fmt.Println("User gid must be 0")
		os.Exit(1)
	}
}

func main() {

}
