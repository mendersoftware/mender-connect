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
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"sync/atomic"
	"syscall"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/mendersoftware/go-lib-micro/ws"
	wsft "github.com/mendersoftware/go-lib-micro/ws/filetransfer"

	"github.com/mendersoftware/mender-connect/config"
	"github.com/mendersoftware/mender-connect/limits/filetransfer"
	"github.com/mendersoftware/mender-connect/session/model"
)

const (
	ACKSlidingWindowSend = 10
	ACKSlidingWindowRecv = 20
	FileTransferBufSize  = 4096
)

var errFileTransferAbort = errors.New("handler aborted")

type FileTransferHandler struct {
	// mutex is used for protecting the async handler. A channel with cap(1)
	// is used instead of sync.Mutex to be able to test acquiring the
	// mutex without blocking.
	mutex chan struct{}
	// msgChan is used to pass messages down to the async file transfer handler routine.
	msgChan chan *ws.ProtoMsg
	permit  *filetransfer.Permit
}

// FileTransfer creates a new filetransfer constructor
func FileTransfer(limits config.Limits) Constructor {
	return func() SessionHandler {
		return &FileTransferHandler{
			mutex:   make(chan struct{}, 1),
			msgChan: make(chan *ws.ProtoMsg),
			permit:  filetransfer.NewPermit(limits),
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
	close(h.msgChan)
	return nil
}

func (h *FileTransferHandler) ServeProtoMsg(msg *ws.ProtoMsg, w ResponseWriter) {
	switch msg.Header.MsgType {
	case wsft.MessageTypePut:
		h.InitFileUpload(msg, w)

	case wsft.MessageTypeStat:
		h.StatFile(msg, w)

	case wsft.MessageTypeGet:
		h.InitFileDownload(msg, w)

	case wsft.MessageTypeACK, wsft.MessageTypeChunk:
		// Messages are digested by async go-routine.
		select {
		// If we can grab the mutex, there are no async handlers running.
		case h.mutex <- struct{}{}:
			<-h.mutex
			h.Error(msg, w, errors.New("no file transfer in progress"))

		case h.msgChan <- msg:
		}

	case wsft.MessageTypeError:
		select {
		// If there's an active async handler, pass the error down,
		// otherwise, log the error.
		case h.mutex <- struct{}{}:
			<-h.mutex
			var erro wsft.Error
			err := msgpack.Unmarshal(msg.Body, &erro)
			if err != nil {
				log.Errorf("Error decoding error message from client: %s", err.Error())
			} else {
				log.Errorf("Received error from client: %s", *erro.Error)
			}
		case h.msgChan <- msg:
		}

	default:
		h.Error(msg, w, errors.Errorf(
			"session: filetransfer message type '%s' not supported",
			msg.Header.MsgType,
		))
	}
}

func (h *FileTransferHandler) StatFile(msg *ws.ProtoMsg, w ResponseWriter) {
	var params model.StatFile
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

// chunkWriter is used for packaging writes into ProtoMsg chunks before
// sending it on the connection.
type chunkWriter struct {
	SessionID string
	Offset    int64
	W         ResponseWriter
}

func (c *chunkWriter) Write(b []byte) (int, error) {
	msg := ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:     ws.ProtoTypeFileTransfer,
			MsgType:   wsft.MessageTypeChunk,
			SessionID: c.SessionID,
			Properties: map[string]interface{}{
				"offset": c.Offset,
			},
		},
		Body: b,
	}
	err := c.W.WriteProtoMsg(&msg)
	if err != nil {
		return 0, err
	}
	c.Offset += int64(len(b))
	return len(b), err
}

func (h *FileTransferHandler) InitFileDownload(msg *ws.ProtoMsg, w ResponseWriter) (err error) {
	params := new(model.GetFile)
	defer func() {
		if err != nil {
			log.Error(err.Error())
			h.Error(msg, w, err)
		}
	}()
	if err = msgpack.Unmarshal(msg.Body, params); err != nil {
		err = errors.Wrap(err, "malformed request parameters")
		return err
	} else if err = params.Validate(); err != nil {
		err = errors.Wrap(err, "invalid request parameters")
		return err
	} else if err = h.permit.DownloadFile(model.FileInfo{
		Path:    params.Path,
		Size:    nil,
		UID:     nil,
		GID:     nil,
		Mode:    nil,
		ModTime: nil,
	}); err != nil {
		log.Warnf("file download access denied: %s", err.Error())
		err = errors.Wrap(err, "access denied")
		return err
	}
	belowLimit := h.permit.BytesSent(uint64(0))
	if !belowLimit {
		log.Warnf("file download tx bytes limit reached.")
		return filetransfer.ErrTxBytesLimitExhausted
	}

	fd, err := os.Open(*params.Path)
	if err != nil {
		err = errors.Wrap(err, "failed to open file for reading")
		return err
	}
	select {
	case h.mutex <- struct{}{}:
		go h.DownloadHandler(fd, msg, w) //nolint:errcheck
	default:
		errClose := fd.Close()
		if errClose != nil {
			log.Warnf("error closing file: %s", err.Error())
		}
		return errors.New("another file transfer is in progress")
	}
	return nil
}

func (h *FileTransferHandler) DownloadHandler(fd *os.File, msg *ws.ProtoMsg, w ResponseWriter) (err error) {
	var (
		ackOffset int64
		N         int64
	)
	defer func() {
		errClose := fd.Close()
		if errClose != nil {
			log.Warnf("error closing file descriptor: %s", errClose.Error())
		}
		if err != nil && err != errFileTransferAbort {
			h.Error(msg, w, err)
			log.Error(err.Error())
		}
		<-h.mutex
	}()

	chunker := &chunkWriter{
		SessionID: msg.Header.SessionID,
		W:         w,
	}

	waitAck := func() (*ws.ProtoMsg, error) {
		msg, open := <-h.msgChan
		if !open {
			return nil, errFileTransferAbort
		}
		switch msg.Header.MsgType {
		case wsft.MessageTypeACK:

		case wsft.MessageTypeError:
			var erro wsft.Error
			msgpack.Unmarshal(msg.Body, &erro) //nolint:errcheck
			if erro.Error != nil {
				log.Errorf("received error message from client: %s", *erro.Error)
			} else {
				log.Error("received malformed error message from client: aborting")
			}
			return msg, errFileTransferAbort

		default:
			return msg, errors.Errorf(
				"received unexpected message type '%s'; expected 'ack'",
				msg.Header.MsgType,
			)
		}
		if off, ok := msg.Header.Properties["offset"]; ok {
			t, ok := off.(int64)
			if !ok {
				return msg, errors.New("invalid offset data type: require int64")
			}
			ackOffset = t
		} else {
			return msg, errors.New("ack message: offset property cannot be blank")
		}
		return msg, nil
	}

	buf := make([]byte, FileTransferBufSize)
	for {
		windowBytes := ackOffset - chunker.Offset +
			ACKSlidingWindowRecv*FileTransferBufSize
		if windowBytes > 0 {
			N, err = io.CopyBuffer(chunker, io.LimitReader(fd, windowBytes), buf)
			if err != nil {
				err = errors.Wrap(err, "failed to copy file chunk to session")
				return err
			}
			belowLimit := h.permit.BytesSent(uint64(N))
			if !belowLimit {
				log.Warnf("file download tx bytes limit reached.")
				return filetransfer.ErrTxBytesLimitExhausted
			}
			if N < windowBytes {
				break
			}
		}

		msg, err = waitAck()
		if err != nil {
			return err
		}
	}

	// Send EOF chunk
	err = w.WriteProtoMsg(&ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:     ws.ProtoTypeFileTransfer,
			MsgType:   wsft.MessageTypeChunk,
			SessionID: msg.Header.SessionID,
			Properties: map[string]interface{}{
				"offset": chunker.Offset,
			},
		},
	})
	if err != nil {
		log.Errorf("failed to send EOF message to client: %s", err.Error())
		return err
	}

	for ackOffset < chunker.Offset {
		msg, err = waitAck()
		if err != nil {
			return err
		}
	}
	return nil
}

func (h *FileTransferHandler) InitFileUpload(msg *ws.ProtoMsg, w ResponseWriter) (err error) {
	var (
		defaultMode uint32 = 0644
		defaultUID  uint32 = uint32(os.Getuid())
		defaultGID  uint32 = uint32(os.Getgid())
	)
	params := model.FileInfo{
		UID:  &defaultUID,
		GID:  &defaultGID,
		Mode: &defaultMode,
	}
	defer func() {
		if err != nil {
			h.Error(msg, w, err)
		}
	}()

	err = msgpack.Unmarshal(msg.Body, &params)
	log.Debugf("InitFileUpload getting upload file: %+v", params)
	if err != nil {
		return errors.Wrap(err, "malformed request parameters")
	} else if err = params.Validate(); err != nil {
		return errors.Wrap(err, "invalid request parameters")
	} else if err = h.permit.UploadFile(params); err != nil {
		return errors.Wrap(err, "access denied")
	}

	belowLimit := h.permit.BytesReceived(uint64(0))
	if !belowLimit {
		log.Warnf("file upload rx bytes limit reached.")
		err = filetransfer.ErrTxBytesLimitExhausted
		return filetransfer.ErrTxBytesLimitExhausted
	}

	select {
	case h.mutex <- struct{}{}:
		go h.FileUploadHandler(msg, params, w) //nolint:errcheck
	default:
		err = errors.New("another file transfer is in progress")
		return err
	}
	return nil
}

var atomicSuffix uint32

func createWrOnlyTempFile(params model.FileInfo) (fd *os.File, err error) {
	for i := 0; i < 100; i++ {
		suffix := atomic.AddUint32(&atomicSuffix, 1)
		filename := *params.Path + fmt.Sprintf(".%08x%02x", suffix, i)
		fd, err = os.OpenFile(filename, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0200)
		if os.IsExist(err) {
			continue
		} else if err != nil {
			break
		}
		break
	}
	if err != nil {
		return nil, errors.Wrap(err, "failed to create file")
	}
	return fd, nil
}

func (h *FileTransferHandler) FileUploadHandler(
	msg *ws.ProtoMsg,
	params model.FileInfo,
	w ResponseWriter,
) (err error) {
	var (
		fd      *os.File
		closeFd bool
	)
	defer func() {
		if fd != nil {
			if closeFd {
				errClose := fd.Close()
				if errClose != nil {
					log.Warnf("error closing file: %s", errClose.Error())
				}
			}
			errRm := os.Remove(fd.Name())
			if errRm != nil {
				log.Errorf(
					"error removing file after aborting upload: %s",
					errRm.Error(),
				)
			}
		}
		if err != nil {
			log.Error(err.Error())
			if errors.Cause(err) != errFileTransferAbort {
				h.Error(msg, w, err)
			}
		}
		<-h.mutex
	}()

	fd, err = createWrOnlyTempFile(params)
	if err != nil {
		h.Error(msg, w, errors.Wrap(err, "failed to create target file"))
		return err
	}
	closeFd = true

	err = w.WriteProtoMsg(&ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:      ws.ProtoTypeFileTransfer,
			MsgType:    wsft.MessageTypeACK,
			SessionID:  msg.Header.SessionID,
			Properties: map[string]interface{}{"offset": int64(0)},
		},
	})
	if err != nil {
		log.Errorf("failed to respond to client: %s", err.Error())
		return errFileTransferAbort
	}

	_, err = h.writeFile(w, fd)
	if err != nil {
		return err
	}
	// Set the final permissions and owner.
	err = fd.Chmod(os.FileMode(*params.Mode) & os.ModePerm)
	if err != nil {
		return errors.Wrap(err, "failed to set file permissions")
	}
	err = fd.Chown(int(*params.UID), int(*params.GID))
	if err != nil {
		return errors.Wrap(err, "failed to set file owner")
	}

	closeFd = false
	filename := fd.Name()
	errClose := fd.Close()
	if errClose != nil {
		log.Warnf("error closing file: %s", errClose.Error())
	}
	err = os.Rename(filename, *params.Path)
	if err != nil {
		return errors.Wrap(err, "failed to commit uploaded file")
	}

	err = h.permit.PreserveOwnerGroup(*params.Path, int(*params.UID), int(*params.GID))
	if err != nil {
		return errors.Wrap(err, "failed to preserve file owner/group "+
			strconv.Itoa(int(*params.UID))+"/"+strconv.Itoa(int(*params.GID)))
	}

	err = h.permit.PreserveModes(*params.Path, os.FileMode(*params.Mode))
	if err != nil {
		return errors.Wrap(err, "failed to preserve file mode "+
			"("+os.FileMode(*params.Mode).String()+")")
	}

	fd = nil
	return err
}

func (h *FileTransferHandler) dstWrite(dst *os.File, body []byte, offset int64) (int, error) {
	n, err := dst.Write(body)
	offset += int64(n)
	belowLimit := h.permit.BytesReceived(uint64(n))
	if !belowLimit || !h.permit.BelowMaxAllowedFileSize(offset) {
		log.Warnf("file upload rx bytes limit reached.")
		return n, filetransfer.ErrTxBytesLimitExhausted
	} else {
		return n, err
	}
}

func (h *FileTransferHandler) writeFile(w ResponseWriter, dst *os.File) (int64, error) {
	var (
		done   bool
		open   bool
		err    error
		i      int
		offset int64
		msg    *ws.ProtoMsg
	)
	// Convenience clojure for decoding file chunk and writing to destination file.
	writeChunk := func(msg *ws.ProtoMsg) error {
		switch msg.Header.MsgType {
		case wsft.MessageTypeChunk:

		case wsft.MessageTypeError:
			var cerr wsft.Error
			packErr := msgpack.Unmarshal(msg.Body, &cerr)
			if packErr == nil && cerr.Error != nil {
				log.Errorf("Received error during upload: %s", *cerr.Error)
			} else {
				log.Error("Received malformed error during upload: aborting")
			}
			return errFileTransferAbort

		default:
			return errors.Errorf(
				"received unexpected message type '%s' during file upload",
				msg.Header.MsgType,
			)
		}
		chunkOffset, ok := msg.Header.Properties["offset"].(int64)
		if !ok {
			return errors.New("invalid file chunk message: missing offset property")
		} else {
			if chunkOffset != offset {
				return errors.New("received unexpected chunk offset")
			}
		}
		if len(msg.Body) > 0 {
			n, err := h.dstWrite(dst, msg.Body, offset)
			offset += int64(n)
			if err != nil {
				return errors.Wrap(err, "failed to write file chunk")
			}
		} else {
			// EOF
			return io.EOF
		}
		return nil
	}

	for !done {
		msg, open = <-h.msgChan
		if !open {
			return offset, errFileTransferAbort
		}
		err = writeChunk(msg)
		if err == io.EOF {
			done = true
		} else if err != nil {
			return offset, err
		}
		// Receive up to ACKSlidingWindowSend file chunks before
		// responding with an ACK.
	InnerLoop:
		for i = 1; i < ACKSlidingWindowSend; i++ {
			runtime.Gosched()
			select {
			case msg, open = <-h.msgChan:
				if !open {
					return offset, errFileTransferAbort
				}
				err = writeChunk(msg)
				if err == io.EOF {
					done = true
				} else if err != nil {
					return offset, err
				}
			default:
				break InnerLoop
			}
		}

		// Copy message headers to response and change message type to ACK.
		rsp := &ws.ProtoMsg{Header: msg.Header}
		rsp.Header.MsgType = wsft.MessageTypeACK
		err = w.WriteProtoMsg(rsp)
		if err != nil {
			log.Errorf("failed to ack file chunk: %s", err.Error())
			return offset, errFileTransferAbort
		}
	}
	return offset, nil
}
