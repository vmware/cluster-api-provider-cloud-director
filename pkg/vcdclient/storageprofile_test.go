package vcdclient

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestStorageProfile(t *testing.T) {
	// get client
	vcdClient, err := getTestVCDClient(
		map[string]interface{}{
			"getVdcClient": true,
		})
	assert.NoError(t, err, "Unable to get VCD client")
	require.NotNil(t, vcdClient, "VCD Client should not be nil")

	storageProfileName := "*"
	storageProfile, err := vcdClient.GetStorageProfileDetailsFromName(storageProfileName)
	assert.NoError(t, err, "should be able to get storage profile [%s]", storageProfileName)
	assert.NotNil(t, storageProfile, "should get storage profile for [%s]", storageProfileName)

	storageProfileName = "not-a-real-profileq-name"
	storageProfile, err = vcdClient.GetStorageProfileDetailsFromName(storageProfileName)
	assert.Error(t, err, "should NOT be able to get random storage profile [%s]", storageProfileName)
	assert.Nil(t, storageProfile, "should NOT get random storage profile for [%s]", storageProfileName)
}
