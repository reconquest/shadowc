package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/docopt/docopt-go"
	"github.com/kovetskiy/executil"
	"github.com/kovetskiy/go-srv"
	"github.com/seletskiy/hierr"
)

var version = "2.0"

const usage = `shadowc, client of login distribution service.

shadowc will request shadow hash entries from specified server and update
shadow file (/etc/shadow) accordingly.

It is capable of requesting users list from shadowd server and creating them,
as well as updating theirs SSH keys (authorized_keys).

Most common invocation is:
    shadowc -KtC -p <pool-name> --all

    This call will request all users from the pool denoted by <pool-name>,
    create them if necessary, request new hash entries, write them into
    /etc/shadow, request SSH keys and overwrite authorized_keys file for each
    user.

    Requests will be sent to addresses which resolves from SRV record _shadowd.

Usage:
    shadowc [options] [-K [-t]] [-C [-g <args>]] [-p <pool>] [-s <addr>...] -u <user>...
    shadowc [options] [-K [-t]] [-C [-g <args>]]  -p <pool>  [-s <addr>...] --all
    shadowc [options] [-K [-t]] [-p <pool>] -s <addr>... --update
    shadowc -v | --version
    shadowc -h | --help

Options:
  -C --create           Create user if it does not exists. User will be created with
                         command 'useradd'. Additional parameters for 'useradd' can be
                         passed using option '-g'.
  -g --useradd <args>   Additional parameters for 'useradd' flag when creating user.
                         If you want to pass several options to 'useradd', specify
                         one flag '-g' with quoted argument.
                         Like that: '-g "-m -Gwheel"'. [default: -m].
  -K --keys             Request SSH keys from shadowd server and append them to the
                         user's authorized_keys file.
  -t --overwrite-keys   Overwrite authorized_keys file instead of appending.
  -s --server <addr>    Use specified login distribution server address.
                         There are several servers can be specified, then shadowc will
                         try to request information from the next server is previous
                         unavailable or do not have required data.
                         Also, SRV name can be specified by using following syntax:
                         _<service>._<proto>.<domain>  or _<service>.
                         [default: _shadowd].
  -p --pool <pool>      Use specified hash tables pool on servers.
  -u --user <user>      Set user which needs shadow entry.
  -a --all              Request all users from specified pull and write shadow entries
                         for them.
  -u --update           Try to update shadow entries for all users from shadow file
                         which already has passwords.
  -c --cert <path>      Set certificate file path [default: /etc/shadowc/cert.pem].
  -f --shadow <file>    Set shadow file path [default: /etc/shadow].
  -w --passwd <passwd>  Set passwd file path (for reading user home dir locations).
                         [default: /etc/passwd]
  --no-srv              Do not try to find shadowd addresses prefixed by '_' in SRV
                         records.
  -h --help             Show this screen.
  -v --version          Show version.
`

func main() {
	args, err := docopt.Parse(usage, nil, true, "shadowc "+version, false)
	if err != nil {
		panic(err)
	}

	var (
		addresses              = args["--server"].([]string)
		shadowFilepath         = args["--shadow"].(string)
		certificateFilepath    = args["--cert"].(string)
		useUsersFromShadowFile = args["--update"].(bool)
		requestUsersFromPool   = args["--all"].(bool)
		canCreateUser          = args["--create"].(bool)
		canUpdateSSHKeys       = args["--keys"].(bool)
		userAddArgs            = args["--useradd"].(string)
		passwdFilePath         = args["--passwd"].(string)
		noSRV                  = args["--no-srv"].(bool)

		shouldOverwriteAuthorizedKeys = args["--overwrite-keys"].(bool)
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
	if args["--pool"] != nil {
		poolName = args["--pool"].(string)
	}

	if !noSRV {
		addresses = tryToResolveSRV(addresses)
	}

	shadowdUpstream, err := NewShadowdUpstream(addresses, certificateFilepath)
	if err != nil {
		log.Fatalln(err)
	}

	var users []string
	switch {
	case useUsersFromShadowFile:
		users, err = getUsersWithPasswords(shadowFilepath)
		if err != nil {
			hierr.Fatalf(
				err, "can't get list of users from %s", shadowFilepath,
			)
		}
	case requestUsersFromPool:
		users, err = getAllUsersFromPool(poolName, shadowdUpstream)
		if err != nil {
			hierr.Fatalf(
				err, "can't get users from pool '%s'", poolName,
			)
		}

		fmt.Printf(
			"Fetched %d entries from pool '%s': %s\n",
			len(users), poolName, strings.Join(users, ", "),
		)

	default:
		users = args["--user"].([]string)
	}

	shadows, err := getShadows(
		users, shadowdUpstream, poolName, useUsersFromShadowFile,
	)
	if err != nil {
		hierr.Fatalf(
			err, "can't get shadows for %s", strings.Join(users, ", "),
		)
	}

	authorizedKeys, err := getAuthorizedKeys(
		users, shadowdUpstream, poolName,
	)
	if err != nil {
		hierr.Fatalf(
			err, "can't get authorized keys for %s", strings.Join(users, ", "),
		)
	}

	shadowFile, err := ReadShadowFile(shadowFilepath)
	if err != nil {
		hierr.Fatalf(
			err, "can't read shadow file %s", shadowFilepath,
		)
	}

	if canCreateUser {
		for _, shadow := range *shadows {
			_, err := shadowFile.GetUserIndex(shadow.User)
			if err != nil {
				fmt.Printf("Creating user '%s'...\n", shadow.User)
				err := createUser(shadow.User, userAddArgs)
				if err != nil {
					hierr.Fatalf(
						err, "can't create user '%s'", shadow.User,
					)
				}
			}
		}

		shadowFile, err = ReadShadowFile(shadowFilepath)
		if err != nil {
			hierr.Fatalf(
				err, "can't read shadow file %s", shadowFilepath,
			)
		}
	}

	if len(*shadows) > 0 {
		fmt.Printf("Writing %d shadow entries...\n", len(*shadows))

		err = writeShadows(shadows, shadowFile)
		if err != nil {
			hierr.Fatalf(
				err, "can't write shadow entries to %s", shadowFilepath,
			)
		}

		fmt.Println("Shadow information updated")
	}

	if canUpdateSSHKeys {
		fmt.Printf("Updating %d SSH keys...\n", len(authorizedKeys))

		addedKeysTotal := 0

		addedKeysTotal, err = writeSSHKeys(
			users, authorizedKeys, passwdFilePath,
			shouldOverwriteAuthorizedKeys,
		)
		if err != nil {
			hierr.Fatalf(
				err, "can't write ssh keys",
			)
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
		return hierr.Errorf(
			err, "can't get temporary file",
		)
	}
	defer temporaryFile.Close()

	for _, shadow := range *shadows {
		err := shadowFile.SetShadow(shadow)
		if err != nil {
			return hierr.Errorf(
				err, "can't set shadow record for user %s", shadow.User,
			)
		}
	}

	_, err = shadowFile.Write(temporaryFile)
	if err != nil {
		return hierr.Errorf(
			err, "can't write temporary file",
		)
	}

	err = temporaryFile.Close()
	if err != nil {
		return hierr.Errorf(
			err, "can't close temporary file",
		)
	}

	err = os.Rename(temporaryFile.Name(), shadowFile.GetPath())
	if err != nil {
		return hierr.Errorf(
			err, "can't rename temporary file to shadow file",
		)
	}

	return nil
}

func writeSSHKeys(
	users []string, keys AuthorizedKeys, passwdFilePath string,
	shouldOverwriteAuthorizedKeys bool,
) (int, error) {
	homeDirs, err := getUsersHomeDirs(passwdFilePath)
	if err != nil {
		return 0, hierr.Errorf(
			err, "can't get users home directories",
		)
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
		err = os.MkdirAll(dir, 0700)
		if err != nil {
			return 0, hierr.Errorf(
				err, "can't create SSH keys directory %s", dir,
			)
		}

		err := changeOwner(user, dir)
		if err != nil {
			return 0, hierr.Errorf(
				err, "can't change directory %s owner to %s", dir, user,
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
				return 0, hierr.Errorf(
					err, "can't read authorized keys file %s", path,
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
		return 0, hierr.Errorf(
			err, "can't craete temporary file in %s", dir,
		)
	}

	_, err = authorizedKeysFile.Write(temporaryFile)
	if err != nil {
		return 0, hierr.Errorf(
			err, "can't write authorized_keys file",
		)
	}

	err = temporaryFile.Close()
	if err != nil {
		return 0, hierr.Errorf(
			err, "can't close temporary file",
		)
	}

	err = changeOwner(user, temporaryFile.Name())
	if err != nil {
		return 0, hierr.Errorf(
			err, "can't change file %s owner to %s",
			temporaryFile.Name(), user,
		)
	}

	err = os.Rename(temporaryFile.Name(), path)
	if err != nil {
		return 0, hierr.Errorf(
			err, "can't rename %s to %s",
			temporaryFile.Name(), path,
		)
	}

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
						user, poolName, shadowdHost.GetAddr(), err,
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
		return []string{}, hierr.Errorf(
			err, "can't read file",
		)
	}

	users := []string{}

	lines := strings.Split(string(contents), "\n")
	for number, line := range lines {
		if line == "" {
			continue
		}

		shadowEntry := strings.Split(line, ":")
		if len(shadowEntry) < 2 {
			return []string{}, fmt.Errorf(
				"invalid shadow entry line #%d: '%s'", number+1, line,
			)
		}

		hash := shadowEntry[1]
		if len(hash) > 1 && hash[0] == '$' {
			users = append(users, shadowEntry[0])
		}
	}

	if len(users) == 0 {
		return nil, errors.New(
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
	_, _, err := executil.Run(exec.Command(
		"sh", "-c", fmt.Sprintf(
			"useradd %s %s",
			userAddArgs,
			userName,
		),
	))
	return err
}

func tryToResolveSRV(records []string) []string {
	addresses := []string{}
	for _, record := range records {
		resolved, err := srv.Resolve(record)
		if err != nil {
			log.Println(err)
			addresses = append(addresses, record)
			continue
		}

		addresses = append(addresses, resolved...)
	}

	return addresses
}

func changeOwner(user, path string) error {
	_, _, err := executil.Run(exec.Command("chown", user+":", path))
	return err
}
