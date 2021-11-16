/*
   Copyright 2021 VMware, Inc.
   SPDX-License-Identifier: Apache-2.0
*/

package vcdclient

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewVCDAuthConfigFromSecrets(t *testing.T) {

	vcdClient, err := getTestVCDClient(nil)
	assert.NoError(t, err, "Unable to get VCD client")
	assert.NotNil(t, vcdClient, "VCD Client should not be nil")

	vcdClient, err = getTestVCDClient(map[string]interface{}{
		"getVdcClient": true,
	})
	assert.NoError(t, err, "Unable to get Client with VDC details.")

	vcdClient, err = getTestVCDClient(map[string]interface{}{
		"host": "https://some-random-address",
	})
	assert.Error(t, err, "Error should be obtained for url [https://some-random-address]")

	vcdClient, err = getTestVCDClient(map[string]interface{}{
		"network": "",
	})
	assert.Error(t, err, "Error should be obtained for missing network")

	vcdClient, err = getTestVCDClient(map[string]interface{}{
		"oneArm": nil,
	})
	assert.NoError(t, err, "There should be no error for missing OneArm")

	return
}
