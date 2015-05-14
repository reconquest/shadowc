package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/docopt/docopt-go"
)

const usage = `shadowc, client of login distribution service.

Usage:
    shadowc [options] [-p <pool>] -s <addr>... -u <user>...
    shadowc [options] [-p <pool>] -s <addr>... --all

Options:
    -s <addr>  Use specified login distribution server address.
    -p <pool>  Use specified hash tables pool on servers.
    -u <user>  Set specified user which needs shadow entry.
    --all      Try to update shadow entries for all users from shadow file which
               already has passwords.
    -c <cert>  Set specified certificate file path [default: /etc/shadowc/cert.pem].
    -f <file>  Set specified shadow file path [default: /etc/shadow].
`

func main() {
	args, err := docopt.Parse(usage, nil, true, "shadowc 1.1", false)
	if err != nil {
		panic(err)
	}

	var (
		addrs               = args["-s"].([]string)
		shadowFilepath      = args["-f"].(string)
		certificateFilepath = args["-c"].(string)
		updateAllMode       = args["--all"].(bool)
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

	var pool string
	if args["-p"] != nil {
		pool = args["-p"].(string)
	}

	var users []string
	if updateAllMode {
		users, err = getUsersWithPasswords(shadowFilepath)
		if err != nil {
			log.Fatal(err.Error())
		}
	} else {
		users = args["-u"].([]string)
	}

	shadows, err := getShadows(
		users, addrs, pool, certificateFilepath, updateAllMode,
	)
	if err != nil {
		log.Fatalln(err)
	}

	fmt.Printf("Writing %d shadow entries...\n", len(*shadows))

	err = writeShadows(shadows, shadowFilepath)
	if err != nil {
		log.Fatalln(err)
	}
}

func writeShadows(shadows *Shadows, shadowFilepath string) error {
	// create temporary file in same directory for preventing 'cross-device
	// link' error.
	temporaryFile, err := ioutil.TempFile(path.Dir(shadowFilepath), "shadow")
	if err != nil {
		return err
	}
	defer temporaryFile.Close()

	shadowEntries, err := ioutil.ReadFile(shadowFilepath)
	if err != nil {
		return err
	}

	lines := strings.Split(
		strings.TrimRight(string(shadowEntries), "\n"),
		"\n",
	)

	for _, shadow := range *shadows {
		found := false
		for lineIndex, line := range lines {
			if strings.HasPrefix(line, shadow.User+":") {
				lines[lineIndex] = fmt.Sprintf("%s", shadow)
				found = true
				break
			}
		}

		if !found {
			return fmt.Errorf(
				"can't found user '%s' in the shadow file", shadow.User,
			)
		}
	}

	_, err = temporaryFile.WriteString(strings.Join(lines, "\n") + "\n")
	if err != nil {
		return err
	}

	err = temporaryFile.Close()
	if err != nil {
		return err
	}

	err = os.Rename(temporaryFile.Name(), shadowFilepath)

	return err
}

func getShadows(
	users []string, addrs []string, pool string,
	certificateFilepath string,
	updateAllMode bool,
) (*Shadows, error) {
	pemData, err := ioutil.ReadFile(certificateFilepath)
	if err != nil {
		return nil, err
	}

	pemBlock, _ := pem.Decode(pemData)
	if pemBlock == nil {
		return nil, fmt.Errorf(
			"%s is not valid certificate file because PEM data is not found",
		)
	}

	certificate, err := x509.ParseCertificate(pemBlock.Bytes)
	if err != nil {
		return nil, err
	}

	certPool := x509.NewCertPool()
	certPool.AddCert(certificate)

	resource := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: certPool,
			},
		},
	}

	shadows := Shadows{}

	for _, user := range users {

		for addrIndex, addr := range addrs {
			repo, _ := NewKeyRepository(addr, resource)

			shadow, err := repo.GetShadow(pool, user)

			if err != nil {
				log.Printf(
					"shadowd host '%s' returned error: %s", addr, err,
				)

				switch err.(type) {
				case HashTableNotFoundError:
					continue

				default:
					addrs = append(addrs[:addrIndex], addrs[addrIndex+1:]...)
					continue
				}
			}

			shadows = append(shadows, shadow)
			break
		}

		if len(addrs) == 0 {
			return nil, fmt.Errorf("all shadowd hosts return errors")
		}
	}

	if len(shadows) == 0 {
		return nil, fmt.Errorf(
			"all shadowd hosts are not aware of '%s' users with '%s' pool",
			strings.Join(users, "', '"),
			pool,
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
		if len(shadowEntry) != 9 {
			return []string{}, fmt.Errorf("invalid shadow entry line: %s", line)
		}

		hash := shadowEntry[1]
		if len(hash) > 1 && hash[0] == '$' {
			users = append(users, shadowEntry[0])
		}
	}

	return users, nil
}
