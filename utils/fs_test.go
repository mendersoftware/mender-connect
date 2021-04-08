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

package utils

import (
	"io/ioutil"
	"os"
	"os/user"
	"path"
	"strconv"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFileSize(t *testing.T) {
	fd, err := ioutil.TempFile("", "tempfile")
	if err != nil {
		t.Fatal("cant create a file")
	}
	fileName := fd.Name()
	fd.Close()
	defer os.Remove(fileName)

	stat, err := os.Stat(fileName)
	if err != nil {
		t.Fatal("cant get file stat")
	}
	expectedFileSize := stat.Size()
	size := FileSize(fileName)
	assert.Equal(t, expectedFileSize, size)

	expectedFileSize = -1
	size = FileSize("lets just say it does not exists" + fileName)
	assert.Equal(t, expectedFileSize, size)
}

func TestFileExists(t *testing.T) {
	fd, err := ioutil.TempFile("", "tempfile")
	if err != nil {
		t.Fatal("cant create a file")
	}
	fileName := fd.Name()
	fd.Close()
	defer os.Remove(fileName)

	expectedExists := true
	exists := FileExists(fileName)
	assert.Equal(t, expectedExists, exists)

	expectedExists = false
	exists = FileExists("lets just say it does not exists" + fileName)
	assert.Equal(t, expectedExists, exists)
}

func TestFileGetUidGid(t *testing.T) {
	fd, err := ioutil.TempFile("", "tempfile")
	if err != nil {
		t.Fatal("cant create a file")
	}
	fileName := fd.Name()
	fd.Close()
	defer os.Remove(fileName)

	stat, err := os.Stat(fileName)
	if err != nil {
		t.Fatal("cant get file stats")
	}
	var statT *syscall.Stat_t
	var ok bool

	if statT, ok = stat.Sys().(*syscall.Stat_t); !ok {
		t.Fatal("cant get file stats")
	}

	expectedUid, expectedGid, err := FileGetUidGid(fileName)
	assert.NoError(t, err)
	assert.Equal(t, expectedUid, statT.Uid)
	assert.Equal(t, expectedGid, statT.Gid)

	expectedUid, expectedGid, err = FileGetUidGid("lets just say it does not exists" + fileName)
	assert.Error(t, err)
}

func TestFileGroupMatches(t *testing.T) {
	fd, err := ioutil.TempFile("", "tempfile")
	if err != nil {
		t.Fatal("cant create a file")
	}
	fileName := fd.Name()
	fd.Close()
	defer os.Remove(fileName)

	stat, err := os.Stat(fileName)
	if err != nil {
		t.Fatal("cant get file stats")
	}
	var statT *syscall.Stat_t
	var ok bool

	if statT, ok = stat.Sys().(*syscall.Stat_t); !ok {
		t.Fatal("cant get file stats")
	}

	group, err := user.LookupGroupId(strconv.Itoa(int(statT.Gid)))
	expectedMatch := FileGroupMatches(fileName, group.Name)
	assert.NoError(t, err)
	assert.True(t, expectedMatch)

	expectedMatch = FileGroupMatches("lets just say it does not exists"+fileName, group.Name)
	assert.False(t, expectedMatch)
}

func TestFileOwnerMatches(t *testing.T) {
	fd, err := ioutil.TempFile("", "tempfile")
	if err != nil {
		t.Fatal("cant create a file")
	}
	fileName := fd.Name()
	fd.Close()
	defer os.Remove(fileName)

	stat, err := os.Stat(fileName)
	if err != nil {
		t.Fatal("cant get file stats")
	}
	var statT *syscall.Stat_t
	var ok bool

	if statT, ok = stat.Sys().(*syscall.Stat_t); !ok {
		t.Fatal("cant get file stats")
	}

	user, err := user.LookupId(strconv.Itoa(int(statT.Uid)))
	expectedMatch := FileOwnerMatches(fileName, user.Username)
	assert.NoError(t, err)
	assert.True(t, expectedMatch)

	expectedMatch = FileOwnerMatches("lets just say it does not exists"+fileName, user.Username)
	assert.False(t, expectedMatch)
}

func TestIsRegularFile(t *testing.T) {
	fd, err := ioutil.TempFile("", "tempfile")
	if err != nil {
		t.Fatal("cant create a file")
	}
	fileName := fd.Name()
	fd.Close()
	defer os.Remove(fileName)

	err = os.Symlink(fileName, fileName+"-link")
	if err != nil {
		t.Fatal("cant create a link")
	}

	isRegular := IsRegularFile(fileName)
	assert.True(t, isRegular)

	isRegular = IsRegularFile("lets just say it does not exists" + fileName)
	assert.False(t, isRegular)

	isRegular = IsRegularFile(fileName + "-link")
	assert.False(t, isRegular)
}

func TestIsInChroot(t *testing.T) {
	chroot := path.Join(os.TempDir(), "chroot")
	if err := os.Mkdir(chroot, 0755); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(chroot)
	fd, err := ioutil.TempFile("", "tempfile")
	if err != nil {
		t.Fatal("cant create a file")
	}
	fileName := fd.Name()
	fd.Close()
	defer os.Remove(fileName)
	fd, err = ioutil.TempFile(chroot, "tempfile")
	if err != nil {
		t.Fatal("cant create a file")
	}
	fd.Close()
	fileNameChroot := fd.Name()
	defer os.Remove(fileNameChroot)

	notInChrootExpected := IsInChroot(fileName, chroot)
	assert.False(t, notInChrootExpected)
	notInChrootExpected = IsInChroot("lets just say it does not exists"+fileName, chroot)
	assert.False(t, notInChrootExpected)
	inChrootExpected := IsInChroot(fileNameChroot, chroot)
	assert.True(t, inChrootExpected)
}
