package main

import (
	"fmt"
	"log"

	"github.com/docopt/docopt-go"
)

type (
	ShadowEntry struct {
		login string
		hash  string
	}

	ShadowEntries []*ShadowEntry
)

func main() {
	args, err := getArgs()
	if err != nil {
		log.Fatal(err.Error())
	}

	//user, err := user.Current()
	//if err != nil {
	//log.Fatal(err.Error())
	//}

	//if user.Gid != "0" {
	//log.Fatal("User gid must be 0")
	//}

	repos := args["-s"].([]string)
	logins := args["-u"].([]string)

	shadows, err := getShadowEntries(logins, repos)
	if err != nil {
		log.Fatal(err.Error())
	}
	log.Printf("%#v", shadows)
	writeShadows(shadows)
}

func getShadowEntries(logins, repoAddrs []string) (*ShadowEntries, error) {
	var err error

	entries := new(ShadowEntries)
	for _, repoAddr := range repoAddrs {
		repo, _ := NewKeyRepository(repoAddr)

		entries, err = repo.GetShadowEntries(logins)
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

func getArgs() (map[string]interface{}, error) {
	usage := `shadowc 0.1

Usage:
	shadowc [-u <login>...] [-s <repo>...]

Options:
    -u <login>    request shadow entry for this login. [default: root]
	-s <repo>        Key repositories (may be distributed)
`

	return docopt.Parse(usage, nil, true, "shadowc 0.1", false)
}
