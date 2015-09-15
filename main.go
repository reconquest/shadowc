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

shadowc will request shadow hash entries from specified server and update
shadow file (/etc/shadow) accordingly.

It is capable of requesting users list from shadowd server and creating them,
as well as updating theirs SSH keys (authorized_keys).

Most common invocation is:
    shadowc -KtC -s <shadowd-addr> -p <pool-name> --all

    This call will request all users from the pool denoted by <pool-name>,
    create them if necessary, request new hash entries, write them into
    /etc/shadow, request SSH keys and overwrite authorized_keys file for each
    user.

Usage:
    shadowc [options] [-K [-t]] [-C [-g <args>]] [-p <pool>] -s <addr>... -u <user>...
    shadowc [options] [-K [-t]] [-C [-g <args>]]  -p <pool>  -s <addr>... --all
    shadowc [options] [-K [-t]] [-p <pool>] -s <addr>... --update

Options:
    -C  Create user if it does not exists. User will be created with
        command 'useradd'. Additional parameters for 'useradd' can be
        passed using options '-g'.
         -g <args>   Additional parameters for 'useradd' flag when creating user.
                     If you want to pass several options to 'useradd', specify
                     one flag '-g' with quoted argument.
                     Like that: '-g "-m -Gwheel"'. [default: -m].
    -K  Request SSH keys from shadowd server and append them to the
        user's authorized_keys file.
         -t          Overwrite authorized_keys file instead of appending.
    -s <addr>        Use specified login distribution server address.
    -p <pool>        Use specified hash tables pool on servers.
    -u <user>        Set user which needs shadow entry.
    --all            Request all users from specified pull and write shadow entries
                     for them.
    --update         Try to update shadow entries for all users from shadow file
                     which already has passwords.
    -c <cert>        Set certificate file path [default: /etc/shadowc/cert.pem].
    -f <file>        Set shadow file path [default: /etc/shadow].
    -w <passwd>      Set passwd file path (for reading user home dir locations).
                     [default: /etc/passwd]
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
		canUpdateSSHKeys       = args["-K"].(bool)
		userAddArgs            = args["-g"].(string)
		passwdFilePath         = args["-w"].(string)

		shouldOverwriteAuthorizedKeys = args["-t"].(bool)
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

	var poolName string
	if args["-p"] != nil {
		poolName = args["-p"].(string)
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
		users, err = getAllUsersFromPool(poolName, shadowdUpstream)
		if err != nil {
			log.Fatal(err.Error())
		}

		fmt.Printf(
			"Fetched %d entries from pool '%s': %s\n",
			len(users),
			poolName,
			strings.Join(users, ", "),
		)

	default:
		users = args["-u"].([]string)
	}

	shadows, err := getShadows(
		users, shadowdUpstream, poolName, useUsersFromShadowFile,
	)
	if err != nil {
		log.Fatalln(err)
	}

	authorizedKeys, err := getAuthorizedKeys(
		users, shadowdUpstream, poolName,
	)
	if err != nil {
		log.Println(err)
	}

	shadowFile, err := ReadShadowFile(shadowFilepath)
	if err != nil {
		log.Fatalln(err)
	}

	if canCreateUser {
		for _, shadow := range *shadows {
			user := shadow.User
			_, err := shadowFile.GetUserIndex(user)
			if err != nil {
				fmt.Printf("Creating user '%s'...\n", user)
				err := createUser(user, userAddArgs)
				if err != nil {
					log.Fatalln(err)
				}
			}
		}

		shadowFile, err = ReadShadowFile(shadowFilepath)
		if err != nil {
			log.Fatalln(err)
		}
	}

	if len(*shadows) > 0 {
		fmt.Printf("Writing %d shadow entries...\n", len(*shadows))

		err = writeShadows(shadows, shadowFile)
		if err != nil {
			log.Fatalln(err)
		}

		fmt.Println("Shadow information updated")
	}

	if canUpdateSSHKeys && len(*shadows) > 0 {
		fmt.Printf("Updating %d SSH keys...\n", len(authorizedKeys))

		addedKeysTotal := 0

		addedKeysTotal, err = writeSSHKeys(
			users, authorizedKeys, passwdFilePath,
			shouldOverwriteAuthorizedKeys,
		)

		if err != nil {
			log.Fatalln(err)
		}

		fmt.Printf(
			"SSH keys updated: %d new, %d already installed\n",
			addedKeysTotal,
			len(authorizedKeys)-addedKeysTotal,
		)
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

func writeSSHKeys(
	users []string, keys AuthorizedKeys, passwdFilePath string,
	shouldOverwriteAuthorizedKeys bool,
) (int, error) {
	homeDirs, err := getUsersHomeDirs(passwdFilePath)
	if err != nil {
		return 0, err
	}

	addedKeysTotal := 0

	for _, user := range users {
		if _, ok := keys[user]; !ok {
			continue
		}

		if _, ok := homeDirs[user]; !ok {
			log.Printf("no home directory found for user '%s'", user)
			continue
		}

		homeDir := homeDirs[user]

		authorizedKeysFilePath := filepath.Join(
			homeDir, ".ssh", "authorized_keys",
		)

		n, err := writeAuthorizedKeysFile(
			user,
			authorizedKeysFilePath, keys[user],
			shouldOverwriteAuthorizedKeys,
		)

		addedKeysTotal += n

		if err != nil {
			log.Printf(
				"can't update user '%s' SSH keys: %s",
				user,
				err,
			)
		}
	}

	return addedKeysTotal, nil
}

func writeAuthorizedKeysFile(
	user string,
	path string, sshKeys SSHKeys,
	shouldOverwrite bool,
) (int, error) {

	dir := filepath.Dir(path)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		err = os.Mkdir(dir, 0700)
		if err != nil {
			return 0, fmt.Errorf(
				"can't create .ssh keys directory '%s': %s",
				dir,
				err,
			)
		}

		output, err := exec.Command("chown", user+":", dir).CombinedOutput()
		if err != nil {
			return 0, fmt.Errorf(
				"error while chowning '%s': %s (%s)",
				dir, output, err,
			)
		}
	}

	var authorizedKeysFile *AuthorizedKeysFile
	if shouldOverwrite {
		authorizedKeysFile = NewAuthorizedKeysFile(path)
	} else {
		var err error

		authorizedKeysFile, err = ReadAuthorizedKeysFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				authorizedKeysFile = NewAuthorizedKeysFile(path)
			} else {
				return 0, fmt.Errorf(
					"can't read '%s': %s",
					path,
					err,
				)
			}
		}
	}

	addedKeysCount := 0
	for _, sshKey := range sshKeys {
		added := authorizedKeysFile.AddSSHKey(sshKey)
		if added {
			addedKeysCount++
			fmt.Printf(
				"SSH key with comment '%s' added to user '%s'\n",
				sshKey.GetComment(),
				user,
			)
		}
	}

	temporaryFile, err := ioutil.TempFile(dir, filepath.Base(dir))
	if err != nil {
		return 0, fmt.Errorf(
			"can't create temp file in '%s': '%s'",
			dir,
			err,
		)
	}

	_, err = authorizedKeysFile.Write(temporaryFile)
	if err != nil {
		return 0, err
	}

	err = temporaryFile.Close()
	if err != nil {
		return 0, err
	}

	err = os.Rename(temporaryFile.Name(), path)

	return addedKeysCount, err
}

func getShadows(
	users []string, shadowdUpstream *ShadowdUpstream, poolName string,
	useUsersFromShadowFile bool,
) (*Shadows, error) {
	shadows := Shadows{}
	for _, user := range users {
		shadowdHosts, err := shadowdUpstream.GetAliveShadowdHosts()
		if err != nil {
			return nil, err
		}

		if len(shadowdHosts) == 0 {
			return nil, fmt.Errorf("no more live servers in shadowd upstream")
		}

		shadowFound := false
		for _, shadowdHost := range shadowdHosts {
			shadow, err := shadowdHost.GetShadow(poolName, user)
			if err != nil {
				switch err.(type) {
				case NotFoundError:
					err := fmt.Errorf(
						"no shadow for user '%s' in pool '%s' on '%s': %s",
						user,
						poolName,
						shadowdHost.GetAddr(),
						err,
					)

					if !useUsersFromShadowFile {
						return nil, err
					}

					log.Println(err)

				default:
					shadowdHost.SetIsAlive(false)

					log.Printf(
						"error retrieving shadows from host '%s': %s",
						shadowdHost.GetAddr(), err.Error(),
					)
				}

				continue
			}

			shadowFound = true
			shadows = append(shadows, shadow)
			break
		}

		if useUsersFromShadowFile && !shadowFound && len(shadowdHosts) > 1 {
			log.Printf(
				"all shadowd hosts are not aware of user '%s' within '%s' pool\n",
				user, poolName,
			)
		}
	}

	if useUsersFromShadowFile && len(shadows) == 0 {
		return nil, fmt.Errorf(
			"no information available for users '%s' from pool '%s'",
			strings.Join(users, "', '"),
			poolName,
		)
	}

	return &shadows, nil
}

func getAuthorizedKeys(
	users []string, shadowdUpstream *ShadowdUpstream, poolName string,
) (AuthorizedKeys, error) {
	keys := make(AuthorizedKeys)

	for _, user := range users {
		shadowdHosts, err := shadowdUpstream.GetAliveShadowdHosts()
		if err != nil {
			return nil, err
		}

		sshKeysFound := false
		for _, shadowdHost := range shadowdHosts {
			userKeys, err := shadowdHost.GetSSHKeys(poolName, user)
			if err != nil {
				switch err.(type) {
				case NotFoundError:
					// pass

				default:
					shadowdHost.SetIsAlive(false)

					log.Printf(
						"error retrieving SSH keys for '%s' from '%s': %s",
						user,
						shadowdHost.GetAddr(),
						err.Error(),
					)
				}

				continue
			}

			sshKeysFound = true
			keys[user] = userKeys
			break
		}

		if !sshKeysFound {
			log.Printf(
				"no ssh keys found for '%s' within pool '%s'",
				user, poolName,
			)
		}
	}

	return keys, nil
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

	if len(shadowdHosts) == 0 {
		return nil, fmt.Errorf("no more live servers in shadowd upstream")
	}

	var tokens []string
	for _, shadowdHost := range shadowdHosts {
		tokens, err = shadowdHost.GetTokens(poolName)
		if err != nil {
			switch err.(type) {
			case NotFoundError:
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
