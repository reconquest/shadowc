package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"strings"
)

type ShadowFile struct {
	path  string
	lines []string
}

func ReadShadowFile(path string) (*ShadowFile, error) {
	shadowEntries, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(
		strings.TrimRight(string(shadowEntries), "\n"),
		"\n",
	)

	return &ShadowFile{
		path:  path,
		lines: lines,
	}, nil
}

func (file *ShadowFile) SetShadow(shadow *Shadow) error {
	userIndex, err := file.GetUserIndex(shadow.User)
	if err != nil {
		return err
	}

	file.lines[userIndex] = fmt.Sprintf("%s", shadow)

	return nil
}

func (file *ShadowFile) GetUserIndex(userName string) (int, error) {
	for lineIndex, line := range file.lines {
		if strings.HasPrefix(line, userName+":") {
			return lineIndex, nil
		}
	}

	return 0, fmt.Errorf(
		"user '%s' is not found in shadow file '%s'", userName, file.path,
	)
}

func (file *ShadowFile) Write(writer io.Writer) (int, error) {
	return io.WriteString(writer, strings.Join(file.lines, "\n")+"\n")
}

func (file *ShadowFile) GetPath() string {
	return file.path
}
