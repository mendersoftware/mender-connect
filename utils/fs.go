// Copyright 2022 Northern.tech AS
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
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

var (
	ErrUnsupported = errors.New("unsupported platform")
)

// Returns ture if the path points to a directory or file inside chroot
// in case chroot is an empty string it is treated as root directory, i.e.:
// all paths are in that chroot
func IsInChroot(path string, chroot string) (bool, error) {
	if chroot == "" {
		return true, nil
	}
	realRoot, err := filepath.EvalSymlinks(chroot)
	if err != nil {
		return false, fmt.Errorf("evaluating root symlinks: %w", err)
	}
	realTarget, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false, fmt.Errorf("evaluating target symlinks: %w", err)
	}
	absRoot, err := filepath.Abs(realRoot)
	if err != nil {
		return false, fmt.Errorf("resolving root: %w", err)
	}
	absTarget, err := filepath.Abs(realTarget)
	if err != nil {
		return false, fmt.Errorf("resolving target: %w", err)
	}

	rel, err := filepath.Rel(absRoot, absTarget)
	if err != nil {
		return false, err
	}

	// rel == ".." or starts with "../" means target escapes root.
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false, nil
	}
	return true, nil
}

func IsRegularFile(path string) bool {
	s, err := os.Lstat(path)
	if err == nil {
		return s.Mode().IsRegular()
	} else {
		return false
	}
}

func FileOwnerMatches(path string, username string) bool {
	if username == "" {
		return true
	}
	uid, _, err := FileGetUidGid(path)
	if err != nil {
		return false
	} else {
		uidString := strconv.Itoa(int(uid))
		fileUser, err := user.LookupId(uidString)
		if err != nil {
			return false
		} else {
			return fileUser.Username == username && fileUser.Uid == uidString
		}
	}
}

func FileGroupMatches(path string, groupName string) bool {
	if groupName == "" {
		return true
	}
	_, gid, err := FileGetUidGid(path)
	if err != nil {
		return false
	} else {
		gidString := strconv.Itoa(int(gid))
		fileGroup, err := user.LookupGroupId(gidString)
		if err != nil {
			return false
		} else {
			return fileGroup.Name == groupName && fileGroup.Gid == gidString
		}
	}
}

func FileGetUidGid(path string) (uint32, uint32, error) {
	stat, err := os.Stat(path)
	if err != nil {
		return 0, 0, err
	}
	if statT, ok := stat.Sys().(*syscall.Stat_t); ok {
		return statT.Uid, statT.Gid, nil
	}
	return 0, 0, ErrUnsupported
}

func FileExists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	} else {
		return false
	}
}

func FileSize(path string) int64 {
	s, err := os.Stat(path)
	if err == nil {
		return s.Size()
	} else {
		return -1
	}
}

func FileModes(path string) os.FileMode {
	s, err := os.Stat(path)
	if err == nil {
		return s.Mode()
	} else {
		return 0
	}
}
