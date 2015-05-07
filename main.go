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
	"strings"

	"github.com/docopt/docopt-go"
)

const usage = `shadowc, client of login distribution service.

Usage:
	shadowc [-f <file>] [-u <user>...] -s <addr>... -c <cert>

Options:
    -u <user> Set specified user which needs shadow entry [default: root]
    -s <addr> Set specified login distribution server address.
    -f <file> Set specified shadow file path [default: /etc/shadow].
    -c <cert> Set specified certificate file path [default: /var/shadowd/cert/cert.pem].
`

func main() {
	args, _ := docopt.Parse(usage, nil, true, "shadowc 1.0", false)

	var (
		addrs               = args["-s"].([]string)
		users               = args["-u"].([]string)
		shadowFilepath      = args["-f"].(string)
		certificateFilepath = args["-c"].(string)
	)

	shadows, err := getShadows(users, addrs, certificateFilepath)
	if err != nil {
		log.Fatal(err)
	}

	err = writeShadows(shadows, shadowFilepath)
	if err != nil {
		panic(err)
	}
}

func writeShadows(shadows *Shadows, shadowFilepath string) (err error) {
	file, err := os.OpenFile(shadowFilepath, os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer file.Close()

	content, err := ioutil.ReadAll(file)
	if err != nil {
		return err
	}

	lines := strings.Split(strings.TrimRight(string(content), "\n"), "\n")

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
			lines = append(lines, fmt.Sprintf("%s", shadow))
		}
	}

	_, err = file.WriteString(strings.Join(lines, "\n") + "\n")

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
		return nil, fmt.Errorf("no PEM data is found in file")
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
			log.Printf("%s", err)

			// try with next repo
			continue
		}
	}

	return nil, fmt.Errorf("repos upstream has gone away")
}
