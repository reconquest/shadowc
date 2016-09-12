package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/reconquest/hierr-go"
)

func getUsersHomeDirs(passwdPath string) (map[string]string, error) {
	file, err := os.Open(passwdPath)
	if err != nil {
		return nil, hierr.Errorf(
			err, "can't open passwd file at %s", passwdPath,
		)
	}

	scanner := bufio.NewScanner(file)

	users := make(map[string]string)

	for scanner.Scan() {
		passwdEntry := strings.Split(scanner.Text(), ":")
		if len(passwdEntry) < 7 {
			return nil, fmt.Errorf(
				"invalid passwd entry encountered in %s: %s",
				passwdPath,
				passwdEntry,
			)
		}

		if passwdEntry[5] == "" || passwdEntry[5] == "/" {
			continue
		}

		users[passwdEntry[0]] = passwdEntry[5]
	}

	return users, nil
}
