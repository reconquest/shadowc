package main

import (
	"bufio"
	"io"
	"os"
	"strings"
)

type SSHKey string
type SSHKeys []SSHKey
type AuthorizedKeys map[string]SSHKeys

type AuthorizedKeysFile struct {
	path string
	keys SSHKeys
}

func (key *SSHKey) GetComment() string {
	parts := strings.Split(string(*key), " ")

	if len(parts) < 3 {
		return ""
	} else {
		return parts[2]
	}
}

func NewAuthorizedKeysFile(path string) *AuthorizedKeysFile {
	return &AuthorizedKeysFile{
		path: path,
	}
}

func ReadAuthorizedKeysFile(path string) (*AuthorizedKeysFile, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(file)

	sshKeys := SSHKeys{}

	for scanner.Scan() {
		sshKeys = append(sshKeys, SSHKey(scanner.Text()))
	}

	return &AuthorizedKeysFile{
		path: path,
		keys: sshKeys,
	}, nil
}

func (file *AuthorizedKeysFile) AddSSHKey(key SSHKey) bool {
	for _, existKey := range file.keys {
		if existKey == key {
			return false
		}
	}

	file.keys = append(file.keys, key)

	return true
}

func (file *AuthorizedKeysFile) Write(writer io.Writer) (int, error) {
	totalWritten := 0

	for _, key := range file.keys {
		written, err := io.WriteString(writer, string(key)+"\n")
		if err != nil {
			return totalWritten, err
		}

		totalWritten += written
	}

	return totalWritten, nil
}

func (file *AuthorizedKeysFile) GetPath() string {
	return file.path
}
