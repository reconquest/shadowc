package main

import (
	"log"

	"github.com/docopt/docopt-go"
)

type (
	ShadowEntry struct {
		username string
		hash     string
		//another fields?
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

	entries := ShadowEntries{}
	for _, username := range args["-u"].([]string) {
		entry, err := getShadowEntry(username)
		if err != nil {
			log.Fatal(err.Error())
		}

		entries = append(entries, entry)
	}

	log.Printf("%#v", entries)
}

func getArgs() (map[string]interface{}, error) {
	usage := `shadowc 0.1

Usage:
	shadowc [-u <username>...]

Options:
    -u <username>    Request shadow entry for this username. [default: root]`

	return docopt.Parse(usage, nil, true, "shadowc 0.1", false)
}

func getShadowEntry(username string) (*ShadowEntry, error) {
	entry := &ShadowEntry{
		username: username,
		hash:     "$1$blah$blah",
	}

	return entry, nil
}
