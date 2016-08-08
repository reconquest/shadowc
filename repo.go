package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"

	"github.com/seletskiy/hierr"
)

type ShadowdHost struct {
	addr     string
	resource *http.Client
	alive    bool
}

type ShadowdUpstream struct {
	hosts []*ShadowdHost
}

type NotFoundError struct {
	error
}

func NewShadowdHost(addr string, resource *http.Client) (*ShadowdHost, error) {
	if strings.Contains(addr, "://") {
		return nil, fmt.Errorf("shadowd host must be in the format host:port")
	}

	shadowdHost := &ShadowdHost{
		addr:     addr,
		resource: resource,
		alive:    true,
	}

	return shadowdHost, nil
}

func (shadowdHost *ShadowdHost) SetIsAlive(alive bool) {
	shadowdHost.alive = alive
}

func (shadowdHost *ShadowdHost) IsAlive() bool {
	return shadowdHost.alive
}

func (shadowdHost *ShadowdHost) GetAddr() string {
	return shadowdHost.addr
}

func (shadowdHost *ShadowdHost) GetShadow(
	pool string, user string,
) (*Shadow, error) {
	var token string

	if pool != "" {
		token = pool + "/" + user
	} else {
		token = user
	}

	hash, err := shadowdHost.getHash(token)
	if err != nil {
		return nil, hierr.Errorf(
			err, "can't get shadow hash for %s", token,
		)
	}

	proofHash, err := shadowdHost.getHash(token)
	if err != nil {
		return nil, hierr.Errorf(
			err, "can't get proofing shadow hash for %s", token,
		)
	}

	if hash == proofHash {
		log.Printf(
			"Warning! Hash for token '%s' was recently requested; "+
				"possible break-in attempt.",
			token,
		)
	}

	shadow := &Shadow{
		User: user,
		Hash: hash,
	}

	return shadow, nil
}

func (shadowdHost *ShadowdHost) GetSSHKeys(
	pool string, user string,
) (SSHKeys, error) {
	var token string

	if pool != "" {
		token = pool + "/" + user
	} else {
		token = user
	}

	body, err := doGet(
		shadowdHost.resource,
		"https://"+shadowdHost.addr+"/ssh/"+token,
	)
	if err != nil {
		return nil, hierr.Errorf(
			err, "request to %s crashed", shadowdHost.addr,
		)
	}

	sshKeys := SSHKeys{}

	rawKeys := strings.Split(strings.TrimRight(body, "\n"), "\n")
	for keyIndex, rawKey := range rawKeys {
		key, err := ReadSSHKey(rawKey)
		if err != nil {
			return nil, hierr.Errorf(
				err, "error while parsing #%d key", keyIndex+1,
			)
		}

		sshKeys = append(sshKeys, key)
	}

	return sshKeys, nil
}

func (shadowdHost *ShadowdHost) getHash(token string) (string, error) {
	body, err := doGet(
		shadowdHost.resource,
		"https://"+shadowdHost.addr+"/t/"+token,
	)
	if err != nil {
		return "", hierr.Errorf(
			err, "request to %s crashed", shadowdHost.addr,
		)
	}

	return strings.TrimRight(body, "\n"), nil
}

func (shadowdHost *ShadowdHost) GetTokens(base string) ([]string, error) {
	body, err := doGet(
		shadowdHost.resource,
		"https://"+shadowdHost.addr+
			"/t/"+strings.TrimSuffix(base, "/")+"/",
	)
	if err != nil {
		return nil, hierr.Errorf(
			err, "request to %s crashed", shadowdHost.addr,
		)
	}

	return strings.Split(body, "\n"), nil
}

func NewShadowdUpstream(
	addrs []string, certificateFilepath string,
) (*ShadowdUpstream, error) {
	pemData, err := ioutil.ReadFile(certificateFilepath)
	if err != nil {
		return nil, hierr.Errorf(
			err, "can't read certificate file %s", certificateFilepath,
		)
	}

	pemBlock, _ := pem.Decode(pemData)
	if pemBlock == nil {
		return nil, fmt.Errorf(
			"%s is not valid certificate file because PEM data is not found",
			certificateFilepath,
		)
	}

	certificate, err := x509.ParseCertificate(pemBlock.Bytes)
	if err != nil {
		return nil, hierr.Errorf(
			err, "can't parse certificate",
		)
	}

	certsPool := x509.NewCertPool()
	certsPool.AddCert(certificate)

	resource := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: certsPool,
			},
		},
	}

	upstream := ShadowdUpstream{}
	for _, addr := range addrs {
		shadowdHost, err := NewShadowdHost(addr, resource)
		if err != nil {
			return nil, hierr.Errorf(
				err, "can't initialize shadowd client for '%s'", addr,
			)
		}

		upstream.hosts = append(upstream.hosts, shadowdHost)
	}

	return &upstream, nil
}

func (upstream *ShadowdUpstream) GetAliveShadowdHosts() (
	[]*ShadowdHost, error,
) {
	hosts := []*ShadowdHost{}
	for _, host := range upstream.hosts {
		if host.IsAlive() {
			hosts = append(hosts, host)
		}
	}

	if len(hosts) < 0 {
		return nil, errors.New("no living shadowd hosts")
	}

	return hosts, nil
}

func readHTTPResponse(response *http.Response) (string, error) {
	if response.StatusCode != 200 {
		if response.StatusCode == 404 {
			return "", NotFoundError{
				errors.New("404 Not Found"),
			}
		}

		return "", fmt.Errorf("unexpected status %s", response.Status)
	}

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return "", hierr.Errorf(
			err, "can't read response body",
		)
	}

	return string(body), nil
}

func doGet(client *http.Client, url string) (string, error) {
	response, err := client.Get(url)
	if err != nil {
		return "", err
	}

	return readHTTPResponse(response)
}
