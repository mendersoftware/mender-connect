// Copyright 2021 Northern.tech AS
//
//    Licensed under the Apache License, Version 2.0 (the "License");
//    you may not use this file except in compliance with the License.
//    You may obtain a copy of the License at
//
//        http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.

package config

import (
	"io"
	"io/ioutil"
	"os"
	"path"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mendersoftware/mender-connect/client/https"
)

const testConfig = `{
  "ServerURL": "https://hosted.mender.io",
  "ServerCertificate": "/var/lib/mender/server.crt",
  "ClientProtocol": "https",
  "HttpsClient": {
    "Certificate": "/data/client.crt",
    "Key": "/data/client.key"
  },
  "User":"root",
  "Terminal": {
    "Height": 80,
    "Width": 24
  },
  "Sessions": {
    "StopExpired": true,
    "ExpireAfter": 16,
    "ExpireAfterIdle": 8,
    "MaxPerUser": 4
  }
}`

const testBrokenConfig = `{
  "ServerURL": "mender
  "ServerCertificate": "/var/lib/mender/server.crt",
  "ClientProtocol": "https",
  "HttpsClient": {
    "Certificate": "/data/client.crt",
    "Key": "/data/client.key"
  }
}`

const testMultipleServersConfig = `{
  "Servers": [
    {"ServerURL": "https://server.one/"},
    {"ServerURL": "https://server.two/"},
    {"ServerURL": "https://server.three/"}
  ]
}`

const testMultipleServersOneNotValidConfig = `{
  "Servers": [
    {"ServerURL": "https://server.one/"},
    {"ServerURL": "%2Casdads://:/sadfa//a"},
    {"ServerURL": "https://server.three/"}
  ]
}`

const testShellIsNotAbsolutePathConfig = `{
		"ShellCommand": "bash",
        "User": "root"
}`

const testShellIsNotExecutablePathConfig = `{
		"ShellCommand": "/etc/profile",
        "User": "root"
}`

const testShellIsNotPresentConfig = `{
		"ShellCommand": "/most/not/here/now",
        "User": "root"
}`

const testUserNotFoundConfig = `{
		"ShellCommand": "/bin/bash",
        "User": "thisoneisnotknown"
}`

const testShellNotInShellsConfig = `{
		"ShellCommand": "/bin/ls",
        "User": "root"
}`

const testParsingErrorConfig = `{
		"ShellCommand": "/bin/ls"
        "User": "root"
}`

const testOtherErrorConfig = `{
		"ShellCommand": 0.0e14,
        "User": "root"
}`

const testTooManyServerDefsConfig = `{
  "ServerURL": "https://hosted.mender.io",
  "ServerCertificate": "/var/lib/mender/server.crt",
  "Servers": [{"ServerURL": "https://hosted.mender.io"}]
}`

const testEmptyServerURL = `{
  "ServerURL": "",
  "User":"root"
}`

func Test_readConfigFile_noFile_returnsError(t *testing.T) {
	err := readConfigFile(nil, "non-existing-file")
	assert.Error(t, err)
}

func Test_readConfigFile_brokenContent_returnsError(t *testing.T) {
	// create a temporary mender-connect.conf file
	tdir, err := ioutil.TempDir("", "mendertest")
	assert.NoError(t, err)
	defer os.RemoveAll(tdir)

	configPath := path.Join(tdir, "mender-connect.conf")
	configFile, err := os.Create(configPath)
	assert.NoError(t, err)

	configFile.WriteString(testBrokenConfig)

	// fail on first call to readConfigFile (invalid JSON)
	confFromFile, err := LoadConfig(configPath, "does-not-exist.config")
	assert.Error(t, err)
	assert.Nil(t, confFromFile)

	// fail on second call to readConfigFile (invalid JSON)
	confFromFile, err = LoadConfig("does-not-exist.config", configPath)
	assert.Error(t, err)
	assert.Nil(t, confFromFile)
}

func validateConfiguration(t *testing.T, actual *MenderShellConfig) {
	expectedConfig := NewMenderShellConfig()
	expectedConfig.MenderShellConfigFromFile = MenderShellConfigFromFile{
		ClientProtocol: "https",
		HTTPSClient: https.Client{
			Certificate: "/data/client.crt",
			Key:         "/data/client.key",
		},
		ServerURL:         "https://hosted.mender.io",
		ServerCertificate: "/var/lib/mender/server.crt",
		Servers:           []https.MenderServer{{ServerURL: "https://hosted.mender.io"}},
		User:              "root",
		ShellCommand:      DefaultShellCommand,
		Terminal: TerminalConfig{
			Width:  24,
			Height: 80,
		},
		Sessions: SessionsConfig{
			StopExpired:     true,
			ExpireAfter:     16,
			ExpireAfterIdle: 8,
			MaxPerUser:      4,
		},
		ReconnectIntervalSeconds: DefaultReconnectIntervalsSeconds,
		Limits: Limits{
			Enabled: false,
			FileTransfer: FileTransferLimits{
				Chroot:         "",
				FollowSymLinks: false,
				AllowOverwrite: false,
				OwnerPut:       "",
				GroupPut:       "",
				Umask:          "",
				MaxFileSize:    0,
				Counters: RateLimits{
					MaxBytesTxPerMinute: 0,
					MaxBytesRxPerMinute: 0,
				},
				AllowSuid:        false,
				RegularFilesOnly: false,
				PreserveMode:     true,
				PreserveOwner:    true,
			},
		},
	}
	if !assert.True(t, reflect.DeepEqual(actual, expectedConfig)) {
		t.Logf("got:      %+v", actual)
		t.Logf("expected: %+v", expectedConfig)
	}
}

func Test_LoadConfig_correctConfFile_returnsConfiguration(t *testing.T) {
	// create a temporary mender-connect.conf file
	tdir, err := ioutil.TempDir("", "mendertest")
	assert.NoError(t, err)
	defer os.RemoveAll(tdir)

	configPath := path.Join(tdir, "mender-connect.conf")
	configFile, err := os.Create(configPath)
	assert.NoError(t, err)
	configFile.WriteString(testConfig)

	// fallback configuration file does not exist
	config, err := LoadConfig(configPath, "does-not-exist.config")
	assert.NoError(t, err)
	assert.NotNil(t, config)
	err = config.Validate()
	assert.NoError(t, err)
	validateConfiguration(t, config)

	httpConfig := config.GetHTTPConfig()
	assert.Equal(t, config.ServerCertificate, httpConfig.ServerCert)
	assert.Equal(t, config.ClientProtocol == httpsSchema, httpConfig.IsHTTPS)
	assert.Equal(t, config.SkipVerify, httpConfig.NoVerify)
	assert.Equal(t, &config.HTTPSClient, httpConfig.Client)

	// main configuration file does not exist
	config2, err2 := LoadConfig("does-not-exist.config", configPath)
	assert.NoError(t, err2)
	assert.NotNil(t, config2)
	err = config2.Validate()
	assert.NoError(t, err)
	validateConfiguration(t, config2)

	httpConfig = config.GetHTTPConfig()
	assert.Equal(t, config2.ServerCertificate, httpConfig.ServerCert)
	assert.Equal(t, config2.ClientProtocol == httpsSchema, httpConfig.IsHTTPS)
	assert.Equal(t, config2.SkipVerify, httpConfig.NoVerify)
	assert.Equal(t, &config2.HTTPSClient, httpConfig.Client)
}

func TestServerURLConfig(t *testing.T) {
	// create a temporary mender-connect.conf file
	tdir, err := ioutil.TempDir("", "mendertest")
	assert.NoError(t, err)
	defer os.RemoveAll(tdir)

	configPath := path.Join(tdir, "mender-connect.conf")
	configFile, err := os.Create(configPath)
	assert.NoError(t, err)

	configFile.WriteString(`{"ServerURL": "https://mender.io/","User":"root"}`)

	// load and validate the configuration
	config, err := LoadConfig(configPath, "does-not-exist.config")
	assert.NoError(t, err)
	err = config.Validate()
	assert.NoError(t, err)
	assert.Equal(t, "https://mender.io", config.Servers[0].ServerURL)

	// not allowed to specify server(s) both as a list and string entry.
	configFile.Seek(0, io.SeekStart)
	configFile.WriteString(testTooManyServerDefsConfig)
	config, err = LoadConfig(configPath, "does-not-exist.config")
	assert.NoError(t, err)
	err = config.Validate()
	assert.Error(t, err)
}

// TestMultipleServersConfig attempts to add multiple servers to config-
// file, as well as overriding the ServerURL from the first server.
func TestMultipleServersConfig(t *testing.T) {
	// create a temporary mender-connect.conf file
	tdir, err := ioutil.TempDir("", "mendertest")
	assert.NoError(t, err)
	defer os.RemoveAll(tdir)

	configPath := path.Join(tdir, "mender-connect.conf")
	configFile, err := os.Create(configPath)
	assert.NoError(t, err)

	configFile.WriteString(testMultipleServersConfig)

	// load config and assert expected values i.e. check that all entries
	// are present and URL's trailing forward slash is trimmed off.
	conf, err := LoadConfig(configPath, "does-not-exist.config")
	assert.NoError(t, err)
	conf.Validate()
	assert.NoError(t, err)
	assert.Equal(t, "https://server.one", conf.Servers[0].ServerURL)
	assert.Equal(t, "https://server.two", conf.Servers[1].ServerURL)
	assert.Equal(t, "https://server.three", conf.Servers[2].ServerURL)
}

func TestEmptyServerURL(t *testing.T) {
	// create a temporary mender-connect.conf file
	tdir, err := ioutil.TempDir("", "mendertest")
	assert.NoError(t, err)
	defer os.RemoveAll(tdir)

	configPath := path.Join(tdir, "mender-connect.conf")
	configFile, err := os.Create(configPath)
	assert.NoError(t, err)

	configFile.WriteString(testEmptyServerURL)

	// load and validate the configuration
	conf, err := LoadConfig(configPath, "does-not-exist.config")
	assert.NoError(t, err)
	err = conf.Validate()
	assert.NoError(t, err)
}

func TestConfigurationMergeSettings(t *testing.T) {
	var mainConfigJSON = `{
		"ShellCommand": "/bin/bash",
		"SkipVerify": true
	}`

	var fallbackConfigJSON = `{
		"ShellCommand": "/bin/sh",
		"User": "mender"
	}`

	mainConfigFile, err := os.Create("main.config")
	assert.NoError(t, err)
	defer os.Remove("main.config")
	mainConfigFile.WriteString(mainConfigJSON)

	fallbackConfigFile, err := os.Create("fallback.config")
	assert.NoError(t, err)
	defer os.Remove("fallback.config")
	fallbackConfigFile.WriteString(fallbackConfigJSON)

	config, err := LoadConfig("main.config", "fallback.config")
	assert.NoError(t, err)
	assert.NotNil(t, config)

	// when a setting appears in neither file, it is left with its default value.
	assert.Equal(t, "", config.ServerCertificate)

	// when a setting appears in both files, the main file takes precedence.
	assert.Equal(t, "/bin/bash", config.ShellCommand)

	// when a setting appears in only one file, its value is used.
	assert.Equal(t, "mender", config.User)
	assert.Equal(t, true, config.SkipVerify)
}

func Test_LoadConfig_various_errors(t *testing.T) {
	// create a temporary mender-connect.conf file
	tdir, err := ioutil.TempDir("", "mendertest")
	assert.NoError(t, err)
	defer os.RemoveAll(tdir)

	//one of the serverURLs is not valid
	configPath := path.Join(tdir, "mender-connect.conf")
	configFile, err := os.Create(configPath)
	assert.NoError(t, err)
	configFile.WriteString(testMultipleServersOneNotValidConfig)

	config, err := LoadConfig(configPath, "")
	assert.NoError(t, err)
	assert.NotNil(t, config)
	err = config.Validate()
	assert.Error(t, err)

	//shell is not an absolute path
	configFile, err = os.Create(configPath)
	assert.NoError(t, err)
	configFile.WriteString(testShellIsNotAbsolutePathConfig)

	config, err = LoadConfig(configPath, "")
	assert.NoError(t, err)
	assert.NotNil(t, config)
	err = config.Validate()
	assert.Error(t, err)

	//shell is not executable
	configFile, err = os.Create(configPath)
	assert.NoError(t, err)
	configFile.WriteString(testShellIsNotExecutablePathConfig)

	config, err = LoadConfig(configPath, "")
	assert.NoError(t, err)
	assert.NotNil(t, config)
	err = config.Validate()
	assert.Error(t, err)

	//shell is not present
	configFile, err = os.Create(configPath)
	assert.NoError(t, err)
	configFile.WriteString(testShellIsNotPresentConfig)

	config, err = LoadConfig(configPath, "")
	assert.NoError(t, err)
	assert.NotNil(t, config)
	err = config.Validate()
	assert.Error(t, err)

	//user not found
	configFile, err = os.Create(configPath)
	assert.NoError(t, err)
	configFile.WriteString(testUserNotFoundConfig)

	config, err = LoadConfig(configPath, "")
	assert.NoError(t, err)
	assert.NotNil(t, config)
	err = config.Validate()
	assert.Error(t, err)

	//shell is not found in /etc/shells
	configFile, err = os.Create(configPath)
	assert.NoError(t, err)
	configFile.WriteString(testShellNotInShellsConfig)

	config, err = LoadConfig(configPath, "")
	assert.NoError(t, err)
	assert.NotNil(t, config)
	err = config.Validate()
	assert.Error(t, err)

	//parsing error
	configFile, err = os.Create(configPath)
	assert.NoError(t, err)
	configFile.WriteString(testParsingErrorConfig)

	config, err = LoadConfig(configPath, "")
	assert.Error(t, err)

	//other loading error
	configFile, err = os.Create(configPath)
	assert.NoError(t, err)
	configFile.WriteString(testOtherErrorConfig)

	config, err = LoadConfig(configPath, "")
	assert.Error(t, err)
}

func TestConfigurationNeitherFileExistsIsNotError(t *testing.T) {
	config, err := LoadConfig("does-not-exist", "also-does-not-exist")
	assert.NoError(t, err)
	assert.IsType(t, &MenderShellConfig{}, config)
}
