// Copyright 2023 Northern.tech AS
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

package filetransfer

import (
	"io/ioutil"
	"math"
	"math/rand"
	"os"
	"os/user"
	"path"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/northerntechhq/nt-connect/config"
	"github.com/northerntechhq/nt-connect/session/model"
)

func TestGetCounters(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	initTX := rand.Uint64()
	initRX := rand.Uint64()
	deviceCounters.bytesTransferred = initTX
	deviceCounters.bytesReceived = initRX
	deviceCounters.bytesTransferredAvg1m = 0.0
	deviceCounters.bytesTransferredAvg1m = 0.0

	time.Sleep(8 * time.Second)
	tx, rx, txm1, rxm1 := GetCounters()
	assert.Equal(t, initTX, tx)
	assert.Equal(t, initRX, rx)
	assert.True(t, math.Abs(txm1) <= 0.001)
	assert.True(t, math.Abs(rxm1) <= 0.001)
}

func TestUpdateCounters(t *testing.T) {
	deviceCounters = Counters{
		bytesTransferred:           0,
		bytesReceived:              0,
		bytesTransferredLastUpdate: time.Now(),
		bytesReceivedLastUpdate:    time.Now(),
	}
	countersUpdateSleepTimeS = 5

	NewPermit(config.Limits{})
	NewPermit(config.Limits{})
	NewPermit(config.Limits{})
	NewPermit(config.Limits{})
	p := NewPermit(config.Limits{
		Enabled: true,
		FileTransfer: config.FileTransferLimits{
			Chroot:         "",
			FollowSymLinks: false,
			AllowOverwrite: false,
			OwnerPut:       "",
			OwnerGet:       []string{},
			Umask:          "",
			MaxFileSize:    0,
			Counters: config.RateLimits{
				MaxBytesTxPerMinute: 0,
				MaxBytesRxPerMinute: 0,
			},
			AllowSuid:        false,
			RegularFilesOnly: false,
		},
	})
	thread1BytesSent := []uint64{
		1024,
		1024,
		1024,
		1024,
		1024,
		1024,
		1024,
		1024,
	}
	thread2BytesReceived := []uint64{
		1024,
		1024,
		1024,
		1024,
		1024,
		1024,
		1024,
		1024,
	}
	thread2BytesSent := []uint64{
		2048,
		2048,
		2048,
		2048,
		2048,
		2048,
		2048,
		2048,
	}
	thread1BytesReceived := []uint64{
		2048,
		2048,
		2048,
		2048,
		2048,
		2048,
		2048,
		2048,
	}
	bytesTxAvg1mExpected := 1663.0
	bytesRxAvg1mExpected := 1663.0
	totalBytesReceivedExpected := uint64(0)
	for _, b := range thread1BytesReceived {
		totalBytesReceivedExpected += b
	}
	for _, b := range thread2BytesReceived {
		totalBytesReceivedExpected += b
	}
	totalBytesSentExpected := uint64(0)
	for _, b := range thread1BytesSent {
		totalBytesSentExpected += b
	}
	for _, b := range thread2BytesSent {
		totalBytesSentExpected += b
	}
	go func() {
		i := 7
		for i >= 0 {
			time.Sleep(50 * time.Millisecond)
			p.BytesSent(thread1BytesSent[i])
			p.BytesReceived(thread1BytesReceived[i])
			i--
		}
	}()
	go func() {
		i := 7
		for i >= 0 {
			time.Sleep(50 * time.Millisecond)
			p.BytesSent(thread2BytesSent[i])
			p.BytesReceived(thread2BytesReceived[i])
			i--
		}
	}()
	time.Sleep(18 * time.Second)
	counterUpdateRunning = false
	t.Logf("after 18s; +%v %.2f vs %.2f=%.2f %.2f vs %.2f=%.2f",
		deviceCounters,
		bytesTxAvg1mExpected, deviceCounters.bytesTransferredAvg1m,
		math.Abs(bytesTxAvg1mExpected-deviceCounters.bytesTransferredAvg1m),
		bytesRxAvg1mExpected, deviceCounters.bytesReceivedAvg1m,
		math.Abs(bytesRxAvg1mExpected-deviceCounters.bytesReceivedAvg1m))
	assert.True(t, math.Abs(bytesTxAvg1mExpected-deviceCounters.bytesTransferredAvg1m) < 0.01*bytesTxAvg1mExpected)
	assert.True(t, math.Abs(bytesRxAvg1mExpected-deviceCounters.bytesReceivedAvg1m) < 0.01*bytesTxAvg1mExpected)
	time.Sleep(2 * time.Second)
	assert.Equal(t, totalBytesSentExpected, deviceCounters.bytesTransferred)
	assert.Equal(t, totalBytesReceivedExpected, deviceCounters.bytesReceived)
	//check that now the updateCounters should not be running, so after 2s the deviceCounters rates should stay the same
}

func createRandomFile(prefix string) string {
	if prefix != "" {
		prefix = path.Join(os.TempDir(), prefix)
		os.Mkdir(prefix, 0755)
	}

	f, err := ioutil.TempFile(prefix, "")
	if err != nil || f == nil {
		return ""
	}
	defer f.Close()
	fileName := f.Name()

	rand.Seed(time.Now().UnixNano())

	maxBytes := 512
	array := make([]byte, rand.Intn(maxBytes))
	for i := range array {
		array[i] = byte(rand.Intn(255))
	}
	f.Write(array)
	f.Close()
	return fileName
}

func TestPermit_PreserveOwnerGroup(t *testing.T) {
	fileName := createRandomFile("")
	if fileName == "" {
		t.Fatal("cant create a file")
	}
	defer os.Remove(fileName)

	u, err := user.Current()
	if err != nil {
		t.Fatal("cant get current user")
	}

	counterUpdateRunning = true //disables the counters update routine
	p := NewPermit(config.Limits{
		Enabled: true,
		FileTransfer: config.FileTransferLimits{
			Chroot:         "",
			FollowSymLinks: false,
			AllowOverwrite: false,
			OwnerPut:       "",
			OwnerGet:       []string{},
			Umask:          "",
			MaxFileSize:    0,
			Counters: config.RateLimits{
				MaxBytesTxPerMinute: 0,
				MaxBytesRxPerMinute: 0,
			},
			AllowSuid:        false,
			RegularFilesOnly: false,
			PreserveMode:     true,
			PreserveOwner:    true,
		},
	})

	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)
	err = p.PreserveOwnerGroup(fileName, uid, gid)
	assert.NoError(t, err)

	stat, err := os.Stat(fileName)
	if err != nil {
		t.Fatal("cant get file stats")
	}
	var statT *syscall.Stat_t
	var ok bool

	if statT, ok = stat.Sys().(*syscall.Stat_t); !ok {
		t.Fatal("cant get file stats")
	}

	assert.Equal(t, uint32(uid), statT.Uid)
	assert.Equal(t, uint32(gid), statT.Gid)
}

func TestPermit_PreserveModes(t *testing.T) {
	fileName := createRandomFile("")
	if fileName == "" {
		t.Fatal("cant create a file")
	}
	defer os.Remove(fileName)

	counterUpdateRunning = true //disables the counters update routine
	p := NewPermit(config.Limits{
		Enabled: true,
		FileTransfer: config.FileTransferLimits{
			Chroot:         "",
			FollowSymLinks: false,
			AllowOverwrite: false,
			OwnerPut:       "",
			OwnerGet:       []string{},
			Umask:          "",
			MaxFileSize:    0,
			Counters: config.RateLimits{
				MaxBytesTxPerMinute: 0,
				MaxBytesRxPerMinute: 0,
			},
			AllowSuid:        false,
			RegularFilesOnly: false,
			PreserveMode:     true,
			PreserveOwner:    true,
		},
	})

	testCases := []struct {
		Name         string
		Umask        string
		Mode         string
		ExpectedMode string
	}{
		{
			Name:         "owner-group-other mode",
			Mode:         "755",
			ExpectedMode: "755",
		},
		{
			Name:         "owner-group-other mode +s",
			Mode:         "4755",
			ExpectedMode: "4755",
		},
		{
			Name:         "owner-group-other mode with umask",
			Umask:        "0202",
			ExpectedMode: "575",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			p.limits.FileTransfer.Umask = tc.Umask
			os.Chmod(fileName, 0)
			mode, _ := strconv.ParseUint(tc.Mode, 8, 32)
			p.PreserveModes(fileName, os.FileMode(mode))

			stat, err := os.Stat(fileName)
			if err != nil {
				t.Fatal("cant get file stats")
			}

			actualMode := stat.Mode()
			if (stat.Mode() & os.ModeSetuid) != 0 {
				actualMode &= os.ModePerm
				actualMode |= syscall.S_ISUID
			} else {
				actualMode &= os.ModePerm
			}

			expectedMode, _ := strconv.ParseUint(tc.ExpectedMode, 8, 32)
			expectedMode &= 07777
			assert.Equal(t, os.FileMode(expectedMode), actualMode)
		})
	}
}

func TestPermit_BelowMaxAllowedFileSize(t *testing.T) {
	p := NewPermit(config.Limits{
		Enabled: true,
		FileTransfer: config.FileTransferLimits{
			Chroot:         "",
			FollowSymLinks: false,
			AllowOverwrite: false,
			OwnerPut:       "",
			OwnerGet:       []string{},
			Umask:          "",
			MaxFileSize:    0,
			Counters: config.RateLimits{
				MaxBytesTxPerMinute: 0,
				MaxBytesRxPerMinute: 0,
			},
			AllowSuid:        false,
			RegularFilesOnly: false,
		},
	})

	testCases := []struct {
		Name               string
		Offset             int64
		MaxAllowedFileSize uint64
		ExpectedBelow      bool
	}{
		{
			Name:               "below the limit",
			Offset:             1024,
			MaxAllowedFileSize: 4096,
			ExpectedBelow:      true,
		},
		{
			Name:               "over the limit",
			Offset:             8192,
			MaxAllowedFileSize: 4096,
			ExpectedBelow:      false,
		},
		{
			Name:               "at the limit",
			Offset:             4096,
			MaxAllowedFileSize: 4096,
			ExpectedBelow:      false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			p.limits.FileTransfer.MaxFileSize = tc.MaxAllowedFileSize
			assert.Equal(t, tc.ExpectedBelow, p.BelowMaxAllowedFileSize(tc.Offset))
		})
	}
}

func TestPermit_DownloadFile(t *testing.T) {
	u, _ := user.Current()
	if u == nil {
		t.Fatal("cant get current user")
	}
	currentUser := u.Username
	currentGroup, _ := user.LookupGroupId(u.Gid)
	if currentGroup == nil {
		t.Fatal("cant get current group")
	}

	testCases := []struct {
		Name             string
		Permit           *Permit
		FilePath         string
		ExpectedDownload error
	}{
		{
			Name: "not a regular file",
			Permit: NewPermit(config.Limits{
				Enabled: true,
				FileTransfer: config.FileTransferLimits{
					RegularFilesOnly: true,
				},
			}),
			FilePath:         os.TempDir(),
			ExpectedDownload: ErrOnlyRegularFilesAllowed,
		},
		{
			Name: "not in a chroot",
			Permit: NewPermit(config.Limits{
				Enabled: true,
				FileTransfer: config.FileTransferLimits{
					Chroot: "/var/chroot/mender/file_transfer",
				},
			}),
			ExpectedDownload: ErrChrootViolation,
		},
		{
			Name: "file owner mismatch",
			Permit: NewPermit(config.Limits{
				Enabled: true,
				FileTransfer: config.FileTransferLimits{
					OwnerGet: []string{"this-is-not-that-owner"},
				},
			}),
			ExpectedDownload: ErrFileOwnerMismatch,
		},
		{
			Name: "file owner match",
			Permit: NewPermit(config.Limits{
				Enabled: true,
				FileTransfer: config.FileTransferLimits{
					OwnerGet:       []string{"this-is-not-that-owner", currentUser},
					FollowSymLinks: true,
				},
			}),
		},
		{
			Name: "file group mismatch",
			Permit: NewPermit(config.Limits{
				Enabled: true,
				FileTransfer: config.FileTransferLimits{
					GroupGet: []string{"this is not that group"},
				},
			}),
			ExpectedDownload: ErrFileGroupMismatch,
		},
		{
			Name: "file group match",
			Permit: NewPermit(config.Limits{
				Enabled: true,
				FileTransfer: config.FileTransferLimits{
					GroupGet:       []string{"this is not that group", currentGroup.Name},
					FollowSymLinks: true,
				},
			}),
		},
		{
			Name: "over the max file size limit in bytes",
			Permit: NewPermit(config.Limits{
				Enabled: true,
				FileTransfer: config.FileTransferLimits{
					MaxFileSize:    1,
					FollowSymLinks: true,
				},
			}),
			ExpectedDownload: ErrFileTooBig,
		},
		{
			Name: "below the max file size limit in bytes",
			Permit: NewPermit(config.Limits{
				Enabled: true,
				FileTransfer: config.FileTransferLimits{
					MaxFileSize:    65536,
					FollowSymLinks: true,
				},
			}),
		},
	}

	path := createRandomFile("")
	if path == "" {
		t.Fatal("cant create a file")
	}
	defer os.Remove(path)

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			filePath := path
			if tc.FilePath != "" {
				filePath = tc.FilePath
			}
			err := tc.Permit.DownloadFile(model.GetFile{
				Path: &filePath,
			})
			if tc.ExpectedDownload != nil {
				assert.EqualError(t, err, tc.ExpectedDownload.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPermit_UploadFile(t *testing.T) {
	u, _ := user.Current()
	if u == nil {
		t.Fatal("cant get current user")
	}
	currentUser := u.Username
	currentGroup, _ := user.LookupGroupId(u.Gid)
	if currentGroup == nil {
		t.Fatal("cant get current group")
	}

	testCases := []struct {
		Name           string
		Permit         *Permit
		FilePath       string
		Modes          os.FileMode
		CreateLinks    bool
		ExpectedUpload error
	}{
		{
			Name: "over the max file size limit in bytes",
			Permit: NewPermit(config.Limits{
				Enabled: true,
				FileTransfer: config.FileTransferLimits{
					FollowSymLinks: true,
					MaxFileSize:    1,
				},
			}),
			ExpectedUpload: ErrFileTooBig,
		},
		{
			Name: "not in a chroot",
			Permit: NewPermit(config.Limits{
				Enabled: true,
				FileTransfer: config.FileTransferLimits{
					Chroot: "/var/chroot/mender/file_transfer",
				},
			}),
			ExpectedUpload: ErrChrootViolation,
		},
		{
			Name: "forbidden to follow links",
			Permit: NewPermit(config.Limits{
				Enabled:      true,
				FileTransfer: config.FileTransferLimits{},
			}),
			CreateLinks:    true,
			ExpectedUpload: ErrFollowLinksForbidden,
		},
		{
			Name: "file exists forbidden to overwrite",
			Permit: NewPermit(config.Limits{
				Enabled: true,
				FileTransfer: config.FileTransferLimits{
					FollowSymLinks: true,
					AllowOverwrite: false,
				},
			}),
			ExpectedUpload: ErrForbiddenToOverwriteFile,
		},
		{
			Name: "file exists allowed to overwrite",
			Permit: NewPermit(config.Limits{
				Enabled: true,
				FileTransfer: config.FileTransferLimits{
					FollowSymLinks: true,
					AllowOverwrite: true,
				},
			}),
		},
		{
			Name: "file exists allowed to overwrite owner mismatch",
			Permit: NewPermit(config.Limits{
				Enabled: true,
				FileTransfer: config.FileTransferLimits{
					FollowSymLinks: true,
					AllowOverwrite: true,
					OwnerPut:       "this is the other one",
				},
			}),
			ExpectedUpload: ErrFileOwnerMismatch,
		},
		{
			Name: "file exists allowed to overwrite owner match",
			Permit: NewPermit(config.Limits{
				Enabled: true,
				FileTransfer: config.FileTransferLimits{
					FollowSymLinks: true,
					AllowOverwrite: true,
					OwnerPut:       currentUser,
				},
			}),
		},
		{
			Name: "file exists allowed to overwrite group mismatch",
			Permit: NewPermit(config.Limits{
				Enabled: true,
				FileTransfer: config.FileTransferLimits{
					FollowSymLinks: true,
					AllowOverwrite: true,
					GroupPut:       "this is the other one",
				},
			}),
			ExpectedUpload: ErrFileGroupMismatch,
		},
		{
			Name: "file exists allowed to overwrite group match",
			Permit: NewPermit(config.Limits{
				Enabled: true,
				FileTransfer: config.FileTransferLimits{
					FollowSymLinks: true,
					AllowOverwrite: true,
					GroupPut:       currentGroup.Name,
				},
			}),
		},
		{
			Name: "suid bit not allowed in modes",
			Permit: NewPermit(config.Limits{
				Enabled: true,
				FileTransfer: config.FileTransferLimits{
					FollowSymLinks: true,
					AllowOverwrite: true,
					AllowSuid:      false,
				},
			}),
			Modes:          os.ModePerm | os.ModeSetuid,
			ExpectedUpload: ErrSuidModeForbidden,
		},
		{
			Name: "suid bit allowed in modes",
			Permit: NewPermit(config.Limits{
				Enabled: true,
				FileTransfer: config.FileTransferLimits{
					FollowSymLinks: true,
					AllowOverwrite: true,
					AllowSuid:      true,
				},
			}),
			Modes: os.ModePerm | os.ModeSetuid,
		},
	}

	filename := createRandomFile("")
	if filename == "" {
		t.Fatal("cant create a file")
	}
	defer os.Remove(filename)

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			filePath := filename
			if tc.FilePath != "" {
				filePath = tc.FilePath
			}
			stat, _ := os.Stat(filePath)
			size := stat.Size()
			fileMode := uint32(stat.Mode())
			if tc.Modes != 0 {
				fileMode = uint32(tc.Modes)
			}

			if tc.CreateLinks {
				rand.Seed(time.Now().UnixNano())
				newTempSubDir := "linktarget" + strconv.Itoa(rand.Int())
				newTempSubDirLink := "linkname" + strconv.Itoa(rand.Int())
				err := os.Mkdir(path.Join(os.TempDir(), newTempSubDir), 0700)
				if err != nil {
					t.Fatal("cant create directory " + os.TempDir() + newTempSubDir + " err:" + err.Error())
				}
				os.Symlink(path.Join(os.TempDir(), newTempSubDir), path.Join(os.TempDir(), newTempSubDirLink))
				filePath = createRandomFile(newTempSubDirLink)
				defer func() {
					os.Remove(filePath)
					os.Remove(os.TempDir() + newTempSubDir)
					os.Remove(os.TempDir() + newTempSubDirLink)
				}()
				if filePath == "" {
					t.Fatal("cant create a file")
				}
			}

			err := tc.Permit.UploadFile(model.UploadRequest{
				Path: &filePath,
				Size: &size,
				Mode: &fileMode,
			})
			if tc.ExpectedUpload != nil {
				assert.EqualError(t, err, tc.ExpectedUpload.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
