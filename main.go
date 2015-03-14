package main

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/docopt/docopt-go"
)

const usage = `shadowc, login distribution client

Usage:
	shadowc [-u <login>...] -s <repo>... [--print] [--output <file>]

Options:
    -u <login>           add login [default: root]
    -s <repo>            add key repository
    --stdin              use stdin instead of /etc/shadow
    --stdout             use stdout instead of /etc/shadow`

func main() {
	args, _ := docopt.Parse(usage, nil, true, "shadowc 1.0", false)

	repos := args["-s"].([]string)
	logins := args["-u"].([]string)

	shadows, err := getShadows(logins, repos)
	if err != nil {
		log.Fatal(err)
	}

	writeShadows(shadows, args["--stdin"].(bool), args["--stdout"].(bool))
}

func writeShadows(
	shadows *Shadows, useStdin bool, useStdout bool,
) (err error) {
	var (
		input    io.Reader
		output   io.Writer
		shadowFd io.ReadWriteCloser
	)

	if !useStdin || !useStdout {
		shadowFd, err = os.Open("/etc/shadow")
		if err != nil {
			return err
		}
		defer shadowFd.Close()
	}

	switch {
	case useStdin:
		input = os.Stdin
	case !useStdin:
		input = shadowFd
	case useStdout:
		input = os.Stdout
	case !useStdout:
		output = shadowFd
	}

	content, err := ioutil.ReadAll(input)
	if err != nil {
		return err
	}

	lines := strings.Split(string(content), "\n")

	for _, shadow := range *shadows {
		found := false
		for lineIndex, line := range lines {
			if strings.HasPrefix(line, shadow.Login+":") {
				lines[lineIndex] = fmt.Sprintf("%s", shadow)
				found = true
				break
			}
		}

		if !found {
			lines = append(lines, fmt.Sprintf("%s", shadow))
		}
	}

	writer := bufio.NewWriter(output)
	if _, err = writer.WriteString(strings.Join(lines, "\n")); err != nil {
		return err
	}

	return writer.Flush()
}

func getShadows(logins, repoAddrs []string) (*Shadows, error) {
	var err error

	entries := new(Shadows)
	for _, repoAddr := range repoAddrs {
		repo, _ := NewKeyRepository(repoAddr)

		entries, err = repo.GetShadows(logins)
		if err == nil {
			return entries, nil
		} else {
			log.Printf("%#v", err)

			// try with next repo
			continue
		}
	}

	return nil, fmt.Errorf("Repos upstream has gone away")
}
