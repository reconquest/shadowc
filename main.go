package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/reconquest/executil-go"
	"github.com/reconquest/srv-go"
	"github.com/kovetskiy/godocs"
	"github.com/kovetskiy/lorg"
	"github.com/reconquest/colorgful"
	"github.com/reconquest/hierr-go"
)

var version = "3.0"
var usage = `shadowc, client of login distribution service.

shadowc will request shadow hash entries from specified server and update
shadow file (/etc/shadow) accordingly.

It is capable of requesting users list from shadowd server and creating them,
as well as updating theirs SSH keys (authorized_keys).

Most common invocation is:
  shadowc -KtC -p <pool> --all

  This call will request all users from the pool denoted by <pool>,
  create them if necessary, request new hash entries, write them into
  /etc/shadow, request SSH keys and overwrite authorized_keys file for each
  user.

  Requests will be sent to addresses which resolves from SRV record _shadowd.

Usage:
  shadowc [options] [-K [-t]] [-C [-g <args>]] [-p <pool>] [-s <addr>...] -u <user>...
  shadowc [options] [-K [-t]] [-C [-g <args>]]  -p <pool>  [-s <addr>...] --all
  shadowc [options] [-K [-t]] [-p <pool>] -s <addr>... --update
  shadowc [options] -P [-s <addr>...] [-p <pool>] -u <user>
  shadowc -v | --version
  shadowc -h | --help

Options:
  -P --password         Generate new hash table for specified user. Will prompt
                         for old and new passwords.
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
  -a --all              Request all users from specified pool and write shadow entries
                         for them.
  -e --update           Try to update shadow entries for all users from shadow file
                         which already has passwords.
  -c --cert <path>      Set certificate file path [default: /etc/shadowc/cert.pem].
  -f --shadow <file>    Set shadow file path [default: /etc/shadow].
  -w --passwd <passwd>  Set passwd file path (for reading user home dir locations).
                         [default: /etc/passwd]
  --no-srv              Do not try to find shadowd addresses prefixed by '_' in SRV
                         records.
  --debug               Show debug messages.
  --trace               Show trace messages.
  -h --help             Show this screen.
  -v --version          Show version.
`

var (
	logger    = lorg.NewLog()
	debugMode = false
	traceMode = false
)

func main() {
	args := godocs.MustParse(usage, version, godocs.UsePager)

	logger.SetIndentLines(true)

	logger.SetFormat(
		colorgful.MustApplyDefaultTheme(
			"${time} ${level:[%s]:right:short} ${prefix}%s",
			colorgful.Dark,
		),
	)

	debugMode = args["--debug"].(bool)
	if debugMode {
		logger.SetLevel(lorg.LevelDebug)
	}

	traceMode = args["--trace"].(bool)
	if traceMode {
		debugMode = true
		logger.SetLevel(lorg.LevelTrace)
	}

	_, err := os.Stat(filepath.Dir(args["--cert"].(string)) + "/key.pem")
	if err == nil {
		fatalf(
			"Key file SHOULD NOT be located on the client machine and " +
				"SHOULD NOT leave shadowd server. " +
				"Please, generate new certificate pair and " +
				"replace certificate file on the clients.",
		)
	}

	addresses := args["--server"].([]string)
	if !args["--no-srv"].(bool) {
		addresses = tryToResolveSRV(addresses)
	}

	upstream, err := NewShadowdUpstream(addresses, args["--cert"].(string))
	if err != nil {
		fatalh(err, "can't initialize shadowd client")
	}

	switch {
	case args["--password"].(bool):
		err = handleChangePassword(upstream, args)

	default:
		err = handlePull(upstream, args)
	}

	if err != nil {
		fatalln(err)
	}
}

func handleChangePassword(
	upstream *ShadowdUpstream, args map[string]interface{},
) error {
	var (
		username = args["--user"].([]string)[0]
		pool, _  = args["--pool"].(string)
	)

	if username == "" {
		return errors.New("username can't be empty")
	}

	oldpassword, err := getPassword("Password: ")
	if err != nil {
		return hierr.Errorf(
			err, "can't prompt for password",
		)
	}

	password, err := getPassword("New password: ")
	if err != nil {
		return hierr.Errorf(
			err, "can't prompt for new password",
		)
	}

	proofPassword, err := getPassword("Repeat new password: ")
	if err != nil {
		return hierr.Errorf(
			err, "can't prompt for repeat new password",
		)
	}

	if proofPassword != password {
		return errors.New("specified passwords do not match")
	}

	if password == "" {
		return errors.New("password can't be empty")
	}

	infof("retrieving shadow salts")

	salts, err := getPasswordChangeSalts(upstream, pool, username)
	if err != nil {
		return hierr.Errorf(
			err, "can't retrieve salts for changing password",
		)
	}

	infof(
		"generating proof shadows using " +
			"specified password and retrieved salts",
	)

	shadows := []string{}
	for _, salt := range salts {
		shadows = append(shadows, crypt(oldpassword, salt))
	}

	infof("requesting hash table generating with new password")

	err = changePassword(upstream, pool, username, shadows, password)
	if err != nil {
		return err
	}

	infof("hash table for %s successfully generated", user{username, pool})

	return nil
}

func handlePull(
	upstream *ShadowdUpstream, args map[string]interface{},
) error {
	var (
		shadowFilepath         = args["--shadow"].(string)
		useUsersFromShadowFile = args["--update"].(bool)
		useUsersFromRemotePool = args["--all"].(bool)
		shouldCreateUser       = args["--create"].(bool)
		shouldUpdateSSHKeys    = args["--keys"].(bool)
		useraddArgs            = args["--useradd"].(string)
		passwdFilePath         = args["--passwd"].(string)
		pool, _                = args["--pool"].(string)

		shouldOverwriteAuthorizedKeys = args["--overwrite-keys"].(bool)
	)

	var usernames []string
	switch {
	case useUsersFromShadowFile:
		infof(
			"retrieving users with passwords from shadow file %s",
			shadowFilepath,
		)

		var err error
		usernames, err = getUsersWithPasswords(shadowFilepath)
		if err != nil {
			return hierr.Errorf(
				err, "can't extract users from shadow file %s", shadowFilepath,
			)
		}

	case useUsersFromRemotePool:
		infof(
			"retrieving users from remote pool %s",
			pool,
		)

		var err error
		usernames, err = getAllUsersFromPool(pool, upstream)
		if err != nil {
			return hierr.Errorf(
				err, "can't retrieve users within pool %s", pool,
			)
		}

	default:
		usernames = args["--user"].([]string)
		for _, username := range usernames {
			if username == "" {
				return errors.New("username can't be empty")
			}
		}
	}

	infof(
		"retrieving shadow entries for %s",
		users{usernames, pool},
	)

	shadows, err := getShadows(
		usernames, upstream, pool, useUsersFromShadowFile,
	)
	if err != nil {
		return hierr.Errorf(err, "can't retrieve shadow entries")
	}

	infof(
		"retrieving ssh keys for %s",
		users{usernames, pool},
	)

	authorizedKeys, err := getAuthorizedKeys(
		usernames, upstream, pool,
	)
	if err != nil {
		return hierr.Errorf(
			err, "can't retrieve authorized keys for %s",
			users{usernames, pool},
		)
	}

	if shouldCreateUser {
		infof("reading shadow file %s", shadowFilepath)

		shadowFile, err := ReadShadowFile(shadowFilepath)
		if err != nil {
			return hierr.Errorf(
				err, "can't read shadow file %s", shadowFilepath,
			)
		}

		for _, shadow := range *shadows {
			_, err := shadowFile.GetUserIndex(shadow.Username)
			if err != nil {
				infof("creating user %s", shadow.Username)

				err := createUser(shadow.Username, useraddArgs)
				if err != nil {
					return hierr.Errorf(
						err, "can't create user %s", shadow.Username,
					)
				}
			}
		}
	}

	if len(*shadows) > 0 {
		infof("reading shadow file %s", shadowFilepath)

		shadowFile, err := ReadShadowFile(shadowFilepath)
		if err != nil {
			return hierr.Errorf(
				err, "can't read shadow file %s", shadowFilepath,
			)
		}

		infof("updating %d shadow entries", len(*shadows))

		err = writeShadows(shadows, shadowFile)
		if err != nil {
			return hierr.Errorf(
				err, "can't write shadow entries to %s", shadowFilepath,
			)
		}

		infof("shadow entries updated", len(*shadows))
	}

	if shouldUpdateSSHKeys {
		infof("updating %d ssh keys", len(authorizedKeys))

		written, err := writeSSHKeys(
			usernames, authorizedKeys, passwdFilePath,
			shouldOverwriteAuthorizedKeys,
		)
		if err != nil {
			return hierr.Errorf(
				err, "can't update ssh keys",
			)
		}

		infof(
			"ssh keys updated: %d new, %d already installed",
			written, len(authorizedKeys)-written,
		)
	}

	return nil
}

func getPasswordChangeSalts(
	upstream *ShadowdUpstream, pool, username string,
) ([]string, error) {
	shadowdHosts, err := upstream.GetAliveShadowdHosts()
	if err != nil {
		return nil, err
	}

	for _, shadowdHost := range shadowdHosts {
		salts, err := shadowdHost.GetPasswordChangeSalts(pool, username)
		if err != nil {
			switch err.(type) {
			case NotFoundError:
				warningf(
					"[%s] is not aware of %s",
					shadowdHost.GetAddr(), user{username, pool},
				)

			default:
				shadowdHost.SetIsAlive(false)

				errorh(
					err,
					"[%s] has gone away", shadowdHost.GetAddr(),
				)
			}

			continue
		}

		return salts, nil
	}

	return nil, fmt.Errorf(
		"no information available for %s in all shadowd servers",
		user{username, pool},
	)
}

func changePassword(
	upstream *ShadowdUpstream,
	pool, username string, shadows []string, password string,
) error {
	shadowdHosts, err := upstream.GetAliveShadowdHosts()
	if err != nil {
		return err
	}

	tracef("shadows: %q", shadows)

	for _, shadowdHost := range shadowdHosts {
		err = shadowdHost.ChangePassword(
			pool, username, shadows, password,
		)
		if err != nil {
			switch err.(type) {
			case NotFoundError:
				warningf(
					"[%s] is not aware of %s",
					shadowdHost.GetAddr(), user{username, pool},
				)

			default:
				shadowdHost.SetIsAlive(false)

				errorh(
					err,
					"[%s] has gone away", shadowdHost.GetAddr(),
				)
			}

			continue
		}

		return nil
	}

	return errors.New("can't change password")
}

func writeShadows(shadows *Shadows, shadowFile *ShadowFile) error {
	// create temporary file in same directory for preventing 'cross-device
	// link' error.
	temporaryFile, err := ioutil.TempFile(
		path.Dir(shadowFile.GetPath()), "shadow",
	)
	if err != nil {
		return hierr.Errorf(
			err, "can't create temporary file",
		)
	}
	defer temporaryFile.Close()

	for _, shadow := range *shadows {
		err := shadowFile.SetShadow(shadow)
		if err != nil {
			return hierr.Errorf(
				err, "can't set shadow record for user %s",
				shadow.Username,
			)
		}
	}

	_, err = shadowFile.Write(temporaryFile)
	if err != nil {
		return hierr.Errorf(
			err, "can't write temporary shadow file",
		)
	}

	err = temporaryFile.Close()
	if err != nil {
		return hierr.Errorf(
			err, "can't close temporary shadow file",
		)
	}

	err = os.Rename(temporaryFile.Name(), shadowFile.GetPath())
	if err != nil {
		return hierr.Errorf(
			err,
			"can't rename temporary file (%s) to shadow file (%s)",
			temporaryFile.Name(), shadowFile.GetPath(),
		)
	}

	return nil
}

func writeSSHKeys(
	usernames []string, keys AuthorizedKeys, passwdFilePath string,
	shouldOverwriteAuthorizedKeys bool,
) (int, error) {
	homeDirs, err := getUsersHomeDirs(passwdFilePath)
	if err != nil {
		return 0, hierr.Errorf(
			err, "can't get users home directories from passwd file %s",
			passwdFilePath,
		)
	}

	total := 0

	for _, user := range usernames {
		key, ok := keys[user]
		if !ok {
			continue
		}

		home, ok := homeDirs[user]
		if !ok {
			infof("no home directory found for user %s, skipping", user)
			continue
		}

		path := filepath.Join(
			home, ".ssh", "authorized_keys",
		)

		written, err := writeAuthorizedKeysFile(
			user, path, key, shouldOverwriteAuthorizedKeys,
		)
		if err != nil {
			return total, hierr.Errorf(
				err, "can't update user %s ssh keys", user,
			)
		}

		total += written
	}

	return total, nil
}

func writeAuthorizedKeysFile(
	user string,
	path string, sshKeys SSHKeys,
	shouldOverwrite bool,
) (int, error) {
	dir := filepath.Dir(path)

	_, err := os.Stat(dir)
	if os.IsNotExist(err) {
		err = os.MkdirAll(dir, 0700)
		if err != nil {
			return 0, hierr.Errorf(
				err, "can't create directory at %s", dir,
			)
		}

		err = changeOwner(user, dir)
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

	added := 0
	for _, sshKey := range sshKeys {
		success := authorizedKeysFile.AddSSHKey(sshKey)
		if success {
			infof(
				"SSH key with comment '%s' added to user %s",
				sshKey.GetComment(), user,
			)

			added++
		}
	}

	temporaryFile, err := ioutil.TempFile(dir, filepath.Base(dir))
	if err != nil {
		return 0, hierr.Errorf(
			err, "can't create temporary file at %s", dir,
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
			err, "can't close authorized_keys temporary file",
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

	return added, nil
}

func getShadows(
	usernames []string, upstream *ShadowdUpstream, pool string,
	useUsersFromShadowFile bool,
) (*Shadows, error) {
	shadows := Shadows{}
	for _, username := range usernames {
		shadowdHosts, err := upstream.GetAliveShadowdHosts()
		if err != nil {
			return nil, err
		}

		shadowFound := false
		for _, shadowdHost := range shadowdHosts {
			shadow, err := shadowdHost.GetShadow(pool, username)
			if err != nil {
				switch err.(type) {
				case NotFoundError:
					warningf(
						"[%s] is not aware of %s",
						shadowdHost.GetAddr(), user{username, pool},
					)

				default:
					shadowdHost.SetIsAlive(false)

					errorh(
						err, "[%s] has gone away", shadowdHost.GetAddr(),
					)
				}

				continue
			}

			shadowFound = true
			shadows = append(shadows, shadow)
			break
		}

		if useUsersFromShadowFile && !shadowFound && len(shadowdHosts) > 1 {
			return &shadows, fmt.Errorf(
				"all shadowd servers are not aware of %s",
				user{username, pool},
			)
		}
	}

	if useUsersFromShadowFile && len(shadows) == 0 {
		return nil, fmt.Errorf(
			"no information available for %s in all shadowd servers",
			users{usernames, pool},
		)
	}

	return &shadows, nil
}

func getAuthorizedKeys(
	usernames []string, upstream *ShadowdUpstream, pool string,
) (AuthorizedKeys, error) {
	keys := make(AuthorizedKeys)

	for _, username := range usernames {
		shadowdHosts, err := upstream.GetAliveShadowdHosts()
		if err != nil {
			return nil, err
		}

		sshKeysFound := false
		for _, shadowdHost := range shadowdHosts {
			userKeys, err := shadowdHost.GetSSHKeys(pool, username)
			if err != nil {
				switch err.(type) {
				case NotFoundError:
					warningf(
						"[%s] is not aware of ssh keys for %s",
						shadowdHost.GetAddr(), user{username, pool},
					)
					continue

				default:
					shadowdHost.SetIsAlive(false)

					errorh(
						err, "[%s] has gone away", shadowdHost.GetAddr(),
					)
				}

				continue
			}

			sshKeysFound = true
			keys[username] = userKeys
			break
		}

		if !sshKeysFound && len(shadowdHosts) > 1 {
			warningf("no ssh keys found for %s", user{username, pool})
		}
	}

	return keys, nil
}

func getUsersWithPasswords(shadowFilepath string) ([]string, error) {
	contents, err := ioutil.ReadFile(shadowFilepath)
	if err != nil {
		return []string{}, hierr.Errorf(
			err, "can't read shadow file %s", shadowFilepath,
		)
	}

	usernames := []string{}

	lines := strings.Split(string(contents), "\n")
	for number, line := range lines {
		if line == "" {
			continue
		}

		shadowEntry := strings.Split(line, ":")
		if len(shadowEntry) < 2 {
			return []string{}, hierr.Errorf(
				errors.New(line),
				"invalid shadow line #%d", number+1,
			)
		}

		hash := shadowEntry[1]
		if len(hash) > 1 && hash[0] == '$' {
			usernames = append(usernames, shadowEntry[0])
		}
	}

	if len(usernames) == 0 {
		return nil, errors.New(
			"shadow file does not contains users with passwords",
		)
	}

	return usernames, nil
}

func getAllUsersFromPool(
	pool string, upstream *ShadowdUpstream,
) ([]string, error) {
	shadowdHosts, err := upstream.GetAliveShadowdHosts()
	if err != nil {
		return nil, err
	}

	var tokens []string
	for _, shadowdHost := range shadowdHosts {
		tokens, err = shadowdHost.GetTokens(pool)
		if err != nil {
			switch err.(type) {
			case NotFoundError:
				warningf(
					"[%s] is not aware about pool %s",
					shadowdHost.GetAddr(), pool,
				)

			default:
				shadowdHost.SetIsAlive(false)

				errorh(
					err, "[%s] has gone away", shadowdHost.GetAddr(),
				)
			}
		}
	}

	if len(tokens) == 0 {
		return nil, fmt.Errorf(
			"no users found within pool %s in all upstream",
			pool,
		)
	}

	return tokens, nil
}

func createUser(name, args string) error {
	_, _, err := executil.Run(
		exec.Command(
			"sh", "-c", fmt.Sprintf("useradd %s %s", args, name),
		),
	)
	return err
}

func tryToResolveSRV(records []string) []string {
	addresses := []string{}
	for _, record := range records {
		if !strings.HasPrefix(record, "_") {
			addresses = append(addresses, record)
			continue
		}

		infof("resolving SRV DNS record %s", record)

		resolved, err := srv.Resolve(record)
		if err != nil {
			errorln(err)
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
