/*
   Copyright 2021 VMware, Inc.
   SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

var (
	gitRoot string = ""
)

func init() {
	gitRoot = os.Getenv("GITROOT")
	if gitRoot == "" {
		// It is okay to panic here as this will be caught during dev
		panic("GITROOT should be set")
	}
}

func TestCloudConfig(t *testing.T) {

	testConfigFilePath := filepath.Join(gitRoot, "testdata/config_test.yaml")
	configReader, err := os.Open(testConfigFilePath)
	assert.NoError(t, err, fmt.Sprintf("Unable to open file [%s]", testConfigFilePath))
	defer func() {
		err = configReader.Close()
		assert.NoError(t, err, "Error closing config file [%s], testConfigFilePath")
	}()

	_, err = ParseCloudConfig(configReader)
	assert.NoError(t, err, "Unable to parse config file")
}
