package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/docopt/docopt-go"
)

const usage = `shadowc, client of login distribution service.

Usage:
    shadowc [options] [-C [-g <args>]] [-p <pool>] -s <addr>... -u <user>...
    shadowc [options] [-C [-g <args>]]  -p <pool>  -s <addr>... --all

    shadowc [options] [-p <pool>] -s <addr>... --update

Options:
    -s <addr>  Use specified login distribution server address.
    -p <pool>  Use specified hash tables pool on servers.
    -u <user>  Set specified user which needs shadow entry.
    -C         Create user if it does not exists. User will be created with
               command 'useradd'. Additional parameters for 'useradd' can be
               passed using options '-g'.
      -g <args>  Additional parameters for 'useradd' flag when creating user.
                 If you want to pass several options to 'useradd', specify
				 one flag '-g' with quoted argument.
				 Like that: '-g "-m -Gwheel"'. [default: -m].
    --all      Request all users from specified pull and write shadow entries
               for them.
    --update   Try to update shadow entries for all users from shadow file
               which already has passwords.
    -c <cert>  Set specified certificate file path [default: /etc/shadowc/cert.pem].
    -f <file>  Set specified shadow file path [default: /etc/shadow].
`

func main() {
	args, err := docopt.Parse(usage, nil, true, "shadowc 1.1", false)
	if err != nil {
		panic(err)
	}

	var (
		addrs                  = args["-s"].([]string)
		shadowFilepath         = args["-f"].(string)
		certificateFilepath    = args["-c"].(string)
		useUsersFromShadowFile = args["--update"].(bool)
		requestUsersFromPool   = args["--all"].(bool)
		canCreateUser          = args["-C"].(bool)
		userAddArgs            = args["-g"].(string)
	)

	certificateDirectory := filepath.Dir(certificateFilepath)
	if _, err := os.Stat(certificateDirectory + "/key.pem"); err == nil {
		log.Fatalln(
			"Key file SHOULD NOT be located on the client and " +
				"SHOULD NOT leave shadowd host. " +
				"Please, generate new certificate pair and " +
				"replace certificate file on the clients.",
		)
	}

	var hashTablesPool string
	if args["-p"] != nil {
		hashTablesPool = args["-p"].(string)
	}

	shadowdUpstream, err := NewShadowdUpstream(addrs, certificateFilepath)
	if err != nil {
		log.Fatalln(err)
	}

	var users []string
	switch {
	case useUsersFromShadowFile:
		users, err = getUsersWithPasswords(shadowFilepath)
		if err != nil {
			log.Fatal(err.Error())
		}
	case requestUsersFromPool:
		users, err = getAllUsersFromPool(hashTablesPool, shadowdUpstream)
		if err != nil {
			log.Fatal(err.Error())
		}

		fmt.Printf(
			"Fetched %d entries from pool '%s': %s\n",
			len(users),
			hashTablesPool,
			strings.Join(users, ", "),
		)

	default:
		users = args["-u"].([]string)
	}

	shadows, err := getShadows(
		users, shadowdUpstream, hashTablesPool, useUsersFromShadowFile,
	)
	if err != nil {
		log.Fatalln(err)
	}

	shadowFile, err := NewShadowFile(shadowFilepath)
	if err != nil {
		log.Fatalln(err)
	}

	if canCreateUser {
		for _, user := range users {
			_, err := shadowFile.GetUserIndex(user)
			if err != nil {
				fmt.Printf("Creating user '%s'...\n", user)
				err := createUser(user, userAddArgs)
				if err != nil {
					log.Fatalln(err)
				}
			}
		}

		shadowFile, err = NewShadowFile(shadowFilepath)
		if err != nil {
			log.Fatalln(err)
		}
	}

	fmt.Printf("Writing %d shadow entries...\n", len(*shadows))

	err = writeShadows(shadows, shadowFile)
	if err != nil {
		log.Fatalln(err)
	}
}

func writeShadows(shadows *Shadows, shadowFile *ShadowFile) error {
	// create temporary file in same directory for preventing 'cross-device
	// link' error.
	temporaryFile, err := ioutil.TempFile(
		path.Dir(shadowFile.GetPath()), "shadow",
	)

	if err != nil {
		return err
	}
	defer temporaryFile.Close()

	for _, shadow := range *shadows {
		err := shadowFile.SetShadow(shadow)
		if err != nil {
			return fmt.Errorf(
				"error while updating shadow for '%s': %s", shadow.User,
				err,
			)
		}
	}

	_, err = shadowFile.Write(temporaryFile)
	if err != nil {
		return err
	}

	err = temporaryFile.Close()
	if err != nil {
		return err
	}

	err = os.Rename(temporaryFile.Name(), shadowFile.GetPath())

	return err
}

func getShadows(
	users []string, shadowdUpstream *ShadowdUpstream, hashTablesPool string,
	useUsersFromShadowFile bool,
) (*Shadows, error) {
	shadows := Shadows{}
	for _, user := range users {
		shadowdHosts, err := shadowdUpstream.GetAliveShadowdHosts()
		if err != nil {
			return nil, err
		}

		shadowFound := false
		for _, shadowdHost := range shadowdHosts {
			shadow, err := shadowdHost.GetShadow(hashTablesPool, user)
			if err != nil {
				switch err.(type) {
				case HashTableNotFoundError:
					if !useUsersFromShadowFile {
						return nil, err
					}

				default:
					shadowdHost.SetIsAlive(false)
				}

				log.Printf(
					"shadowd host '%s' returned error: %s",
					shadowdHost.GetAddr(), err.Error(),
				)

				continue
			}

			shadowFound = true
			shadows = append(shadows, shadow)
			break
		}

		if useUsersFromShadowFile && !shadowFound {
			log.Printf(
				"all shadowd hosts are not aware of user '%s' within '%s' pool\n",
				user, hashTablesPool,
			)
		}
	}

	if useUsersFromShadowFile && len(shadows) == 0 {
		return nil, fmt.Errorf(
			"all shadowd hosts are not aware of '%s' users within '%s' pool",
			strings.Join(users, "', '"),
			hashTablesPool,
		)
	}

	return &shadows, nil
}

func getUsersWithPasswords(shadowFilepath string) ([]string, error) {
	contents, err := ioutil.ReadFile(shadowFilepath)
	if err != nil {
		return []string{}, err
	}

	users := []string{}

	lines := strings.Split(string(contents), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		shadowEntry := strings.Split(line, ":")
		if len(shadowEntry) < 2 {
			return []string{}, fmt.Errorf(
				"invalid shadow entry line: %s", line,
			)
		}

		hash := shadowEntry[1]
		if len(hash) > 1 && hash[0] == '$' {
			users = append(users, shadowEntry[0])
		}
	}

	if len(users) == 0 {
		return nil, fmt.Errorf(
			"shadow file is empty",
		)
	}

	return users, nil
}

func getAllUsersFromPool(
	poolName string, shadowdUpstream *ShadowdUpstream,
) ([]string, error) {
	shadowdHosts, err := shadowdUpstream.GetAliveShadowdHosts()
	if err != nil {
		return nil, err
	}

	var tokens []string
	for _, shadowdHost := range shadowdHosts {
		tokens, err = shadowdHost.GetTokens(poolName)
		if err != nil {
			switch err.(type) {
			case TokensListNotFoundError:
				log.Printf(
					"shadowd host '%s' returned error: %s",
					shadowdHost.GetAddr(), err.Error(),
				)
				continue
			default:
				return nil, err
			}
		}
	}

	if len(tokens) == 0 {
		return nil, fmt.Errorf(
			"no tokens found under '%s' in all upstream",
			poolName,
		)
	}

	return tokens, nil
}

func createUser(userName string, userAddArgs string) error {
	createCommand := exec.Command(
		"sh", "-c", fmt.Sprintf(
			"useradd %s %s",
			userAddArgs,
			userName,
		),
	)

	output, err := createCommand.CombinedOutput()
	if err != nil {
		return fmt.Errorf(
			"useradd exited with error: %s",
			output,
		)
	}

	return nil
}
