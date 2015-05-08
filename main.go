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
	"path/filepath"
	"strings"

	"github.com/docopt/docopt-go"
)

const usage = `shadowc, client of login distribution service.

Usage:
	shadowc [-c <cert>] [-f <file>] [-u <user>...] -s <addr>...

Options:
    -u <user>  Set specified user which needs shadow entry [default: root]
    -s <addr>  Set specified login distribution server address.
    -f <file>  Set specified shadow file path [default: /etc/shadow].
    -c <cert>  Set specified certificate file path [default: /etc/shadowc/cert.pem].
`

func main() {
	args, _ := docopt.Parse(usage, nil, true, "shadowc 1.0", false)

	var (
		addrs               = args["-s"].([]string)
		users               = args["-u"].([]string)
		shadowFilepath      = args["-f"].(string)
		certificateFilepath = args["-c"].(string)
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

	shadows, err := getShadows(users, addrs, certificateFilepath)
	if err != nil {
		log.Fatalln(err)
	}

	err = writeShadows(shadows, shadowFilepath)
	if err != nil {
		log.Fatalln(err)
	}
}

func writeShadows(shadows *Shadows, shadowFilepath string) error {
	temporaryFile, err := ioutil.TempFile(os.TempDir(), "shadowc")
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
	users, addrs []string, certificateFilepath string,
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

	for _, addr := range addrs {
		repo, _ := NewKeyRepository(addr, resource)

		shadows, err := repo.GetShadows(users)
		if err == nil {
			return shadows, nil
		} else {
			log.Printf("shadowd host '%s' returned error: %s", addr, err)

			// try with next repo
			continue
		}
	}

	return nil, fmt.Errorf("all shadowd hosts return errors")
}
