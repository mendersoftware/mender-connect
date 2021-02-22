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

package session

import (
	"io"
	"os"
	"syscall"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/vmihailenco/msgpack"

	"github.com/mendersoftware/go-lib-micro/ws"
	wsft "github.com/mendersoftware/go-lib-micro/ws/filetransfer"
)

type FileInfo wsft.FileInfo

func (f FileInfo) Validate() error {
	return validation.ValidateStruct(&f,
		validation.Field(&f.Path, validation.Required),
	)
}

type StatFile wsft.StatFile

func (s StatFile) Validate() error {
	return validation.ValidateStruct(&s,
		validation.Field(&s.Path, validation.Required),
	)
}

type GetFile wsft.GetFile

func (f GetFile) Validate() error {
	return validation.ValidateStruct(&f,
		validation.Field(&f.Path, validation.Required),
	)
}

type FileTransferHandler struct {
	file    *os.File
	filePos int64

	// use channels with cap 1 as mutexes since you can't try a sync.Mutex
	putMutex chan struct{}
	getMutex chan struct{}
}

func FileTransfer() Constructor {
	return func() SessionHandler {
		return &FileTransferHandler{
			putMutex: make(chan struct{}, 1),
			getMutex: make(chan struct{}, 1),
		}
	}
}

func (h *FileTransferHandler) Error(msg *ws.ProtoMsg, w ResponseWriter, err error) {
	errMsg := err.Error()
	msgErr := wsft.Error{
		Error:       &errMsg,
		MessageType: &msg.Header.MsgType,
	}
	rsp := *msg
	rsp.Header.MsgType = wsft.MessageTypeError
	rsp.Body, _ = msgpack.Marshal(msgErr)
	w.WriteProtoMsg(&rsp) //nolint:errcheck
}

func (h *FileTransferHandler) Close() error {
	if h.file != nil {
		err := errors.New("session: filetransfer closed unexpectedly")
		filename := h.file.Name()
		h.file.Close() //nolint:errcheck
		errRm := os.Remove(filename)
		if errRm != nil {
			err = errors.Wrapf(err,
				"failed to remove incomplete file '%s': %s",
				filename, errRm.Error(),
			)
		}
		h.file = nil
		h.filePos = 0
		return err
	}
	return nil
}

func (h *FileTransferHandler) ServeProtoMsg(msg *ws.ProtoMsg, w ResponseWriter) {
	switch msg.Header.MsgType {
	case wsft.MessageTypePut:
		h.InitFileUpload(msg, w)

	case wsft.MessageTypeChunk:
		if h.file == nil {
			h.Error(msg, w, errors.New("session: no file transfer in progress"))
			return
		}
		offset, ok := msg.Header.Properties["offset"].(int64)
		if !ok {
			// Append to file
			offset = int64(-1)
		}
		if len(msg.Body) > 0 {
			err := h.WriteOffset(msg.Body, offset)
			if err != nil {
				h.Error(msg, w, err)
				return
			}
		} else {
			// EOF
			err := h.file.Close()
			if err != nil {
				h.Error(msg, w, errors.Wrap(err, "error completing file transfer"))
			}
			h.file = nil
			h.filePos = 0
			select {
			case <-h.putMutex:
			default:
				// Avoid deadlock if EOF is sent twice
			}
		}

	case wsft.MessageTypeStat:
		h.StatFile(msg, w)

	case wsft.MessageTypeGet:
		go h.GetFile(msg, w)

	default:
		h.Error(msg, w, errors.Errorf(
			"session: filetransfer message type '%s' not supported",
			msg.Header.MsgType,
		))
	}
}

func (h *FileTransferHandler) GetFile(msg *ws.ProtoMsg, w ResponseWriter) {
	const BufSize = 4096
	var params GetFile
	if err := msgpack.Unmarshal(msg.Body, &params); err != nil {
		h.Error(msg, w, errors.Wrap(err, "malformed request parameters"))
		return
	} else if err = params.Validate(); err != nil {
		h.Error(msg, w, errors.Wrap(err, "invalid request parameters"))
		return
	}
	select {
	case h.getMutex <- struct{}{}:
		defer func() { <-h.getMutex }()
	default:
		h.Error(msg, w, errors.New("another file transfer is still in progress"))
		return
	}

	fd, err := os.Open(*params.Path)
	if err != nil {
		h.Error(msg, w, errors.Wrap(err, "failed to open file for reading"))
		return
	}
	defer fd.Close() //nolint:errcheck

	var offset int64
	buf := make([]byte, BufSize)
	for {
		n, err := fd.Read(buf)
		if err == io.EOF {
			break
		} else if err != nil {
			h.Error(msg, w, errors.Wrap(err, "failed to read file chunk"))
			err = fd.Close() //nolint:errcheck
			if err != nil {
				log.Warnf("failed to close open file descriptor: %s", err.Error())
			}
			return
		}

		err = h.sendChunk(msg, buf[:n], offset, w)
		if err != nil {
			return
		}

		offset += int64(n)
	}

	h.sendChunk(msg, nil, offset, w) //nolint:errcheck
}

func (h *FileTransferHandler) sendChunk(
	req *ws.ProtoMsg,
	b []byte,
	offset int64,
	w ResponseWriter,
) error {
	err := w.WriteProtoMsg(&ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:     ws.ProtoTypeFileTransfer,
			MsgType:   wsft.MessageTypeChunk,
			SessionID: req.Header.SessionID,
			Properties: map[string]interface{}{
				"offset": offset,
			},
		},
		Body: b,
	})
	if err != nil {
		h.Error(req, w, errors.Wrap(err,
			"failed to write file chunk to stream: abort",
		))
	}
	return err
}

func (h *FileTransferHandler) StatFile(msg *ws.ProtoMsg, w ResponseWriter) {
	var params StatFile
	err := msgpack.Unmarshal(msg.Body, &params)
	if err != nil {
		h.Error(msg, w, errors.Wrap(err, "malformed request parameters"))
		return
	} else if err = params.Validate(); err != nil {
		h.Error(msg, w, errors.Wrap(err, "invalid request parameters"))
		return
	}
	stat, err := os.Stat(*params.Path)
	if err != nil {
		h.Error(msg, w, errors.Wrapf(err,
			"failed to get file info from path '%s'", *params.Path))
		return
	}
	mode := uint32(stat.Mode())
	size := stat.Size()
	modTime := stat.ModTime()
	fileInfo := wsft.FileInfo{
		Path:    params.Path,
		Size:    &size,
		Mode:    &mode,
		ModTime: &modTime,
	}
	if statT, ok := stat.Sys().(*syscall.Stat_t); ok {
		// Only return UID/GID if the filesystem/OS supports it
		fileInfo.UID = &statT.Uid
		fileInfo.GID = &statT.Gid
	}
	b, _ := msgpack.Marshal(fileInfo)

	err = w.WriteProtoMsg(&ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:     ws.ProtoTypeFileTransfer,
			MsgType:   wsft.MessageTypeFileInfo,
			SessionID: msg.Header.SessionID,
		},
		Body: b,
	})
	if err != nil {
		log.Errorf("error sending FileInfo to client: %s", err.Error())
	}
}

func (h *FileTransferHandler) WriteOffset(b []byte, offset int64) (err error) {
	defer func() {
		if err != nil {
			filename := h.file.Name()
			h.file.Close() //nolint:errcheck
			h.file = nil
			h.filePos = 0
			errRm := os.Remove(filename)
			if errRm != nil {
				log.Warn("failed to remove file after failed write: " +
					err.Error())
			}
			select {
			case <-h.putMutex:
			default:
			}
		}
	}()
	if offset >= 0 {
		if h.filePos != offset {
			h.filePos, err = h.file.Seek(offset, io.SeekStart)
			if err != nil {
				return errors.Wrap(err, "session: failed to seek to file offset")
			} else if h.filePos != offset {
				return errors.New("session: failed to seek to file offset")
			}
		}
	}
	n, err := h.file.Write(b)
	h.filePos += int64(n)
	if err != nil {
		return errors.Wrap(err, "session: failed to write file chunk")
	}
	return nil
}

func (h *FileTransferHandler) InitFileUpload(msg *ws.ProtoMsg, w ResponseWriter) (err error) {
	select {
	case h.putMutex <- struct{}{}:
	default:
		err = errors.New("session: file upload already in progress")
		h.Error(msg, w, err)
		return err
	}
	var (
		defaultMode uint32 = 0644
		defaultUID  uint32 = uint32(os.Getuid())
		defaultGID  uint32 = uint32(os.Getgid())
	)
	params := FileInfo{
		UID:  &defaultUID,
		GID:  &defaultGID,
		Mode: &defaultMode,
	}
	defer func() {
		if err != nil {
			h.Error(msg, w, err)
			if h.file != nil {
				fileName := h.file.Name()
				h.file.Close()
				os.Remove(fileName)
				h.file = nil
			}
			<-h.putMutex
		}
	}()
	err = msgpack.Unmarshal(msg.Body, &params)
	if err != nil {
		return errors.Wrap(err, "session: malformed request body")
	} else if err = params.Validate(); err != nil {
		return errors.Wrap(err, "session: invalid request parameters")
	}
	h.file, err = os.OpenFile(
		*params.Path,
		os.O_CREATE|os.O_WRONLY,
		os.FileMode(*params.Mode),
	)
	if err != nil {
		return errors.Wrap(err, "session: failed to create file")
	}
	err = h.file.Chown(int(*params.UID), int(*params.GID))
	if err != nil {
		return errors.Wrap(err, "session: failed to set file permissions")
	}
	rsp := *msg
	rsp.Header.MsgType = wsft.MessageTypeContinue
	rsp.Body = nil
	err = w.WriteProtoMsg(&rsp)
	if err != nil {
		log.Error("session: failed to respond to client")
	}
	return nil
}
