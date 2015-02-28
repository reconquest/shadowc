package main

import (
	"fmt"
	"log"

	"github.com/docopt/docopt-go"
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

	shadows, err := getShadows(logins, repos)
	if err != nil {
		log.Fatal(err.Error())
	}

	switch {
	case args["--print"]:
		fmt.Printf("%s", shadows)
	default:
		writeShadows(shadows)
	}

}

func writeShadows(shadows *Shadows) error  {

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


func getArgs() (map[string]interface{}, error) {
	usage := `shadowc 0.1

Usage:
	shadowc [-u <login>...] [-s <repo>...] [--print]

Options:
    -u <login>    add login [default: root]
    -s <repo>     add key repository (may be distributed)
    --print       print resulting <login;hash>
`

	return docopt.Parse(usage, nil, true, "shadowc 0.1", false)
}
