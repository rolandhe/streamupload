package streamupload

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
)

type stepEnum int

const (
	readFirstSegStep    = stepEnum(0)
	readFileContentStep = stepEnum(1)
	readLastSegStep     = stepEnum(2)
	readFinish          = stepEnum(-1)
)

const defaultReadBuffSize = 16 * 1024

type streamUpload struct {
	file       *os.File
	fileReader *bufio.Reader
	firstSeg   bytes.Buffer
	lastSeg    bytes.Buffer
	step       stepEnum
}

func (sup *streamUpload) Close() error {
	if sup.file != nil {
		err := sup.file.Close()
		sup.file = nil
		return err
	}
	return nil
}

func (sup *streamUpload) addFile(fileParamName string, filename string, fieldParams map[string]string) (string, error) {
	var err error
	if sup.file, err = os.Open(filename); err != nil {
		return "", err
	}
	sup.fileReader = bufio.NewReaderSize(sup.file, defaultReadBuffSize)
	w := multipart.NewWriter(&sup.firstSeg)
	_, err = w.CreateFormFile(fileParamName, filepath.Base(filename))
	if err != nil {
		sup.Close()
		return "", err
	}
	formDataContentType := w.FormDataContentType()
	boundary := w.Boundary()

	sup.lastSeg.WriteString("\r\n")
	w = multipart.NewWriter(&sup.lastSeg)
	w.SetBoundary(boundary)
	for key, val := range fieldParams {
		err = w.WriteField(key, val)
		if err != nil {
			sup.Close()
			return "", err
		}
	}
	w.Close()
	return formDataContentType, err
}

func (sup *streamUpload) Read(p []byte) (n int, err error) {
	bufLen := len(p)
	gotLen := 0
	buff := p
	if sup.step == readFirstSegStep {
		n, err = sup.firstSeg.Read(buff)
		if err != nil {
			return n, err
		}
		if n > 0 {
			gotLen += n
			if gotLen == bufLen {
				return gotLen, nil
			}
			sup.step = readFileContentStep
		} else {
			sup.step = readFileContentStep
		}
	}

	if gotLen > 0 {
		buff = p[gotLen:bufLen]
	}
	if sup.step == readFileContentStep {
		n, err = sup.fileReader.Read(buff)
		if err != nil && err != io.EOF {
			return 0, err
		}
		if n > 0 {
			gotLen += n
			if gotLen == bufLen {
				return gotLen, nil
			}
			sup.step = readLastSegStep
		}
		if err == io.EOF || n == 0 {
			sup.step = readLastSegStep
		}
	}

	if gotLen > 0 {
		buff = p[gotLen:bufLen]
	}

	if sup.step == readLastSegStep {
		n, err = sup.lastSeg.Read(buff)
		if err != nil {
			return n, err
		}
		if n > 0 {
			gotLen += n
			if gotLen < bufLen {
				sup.step = readFinish
				return gotLen, io.EOF
			}
			return gotLen, nil
		} else {
			sup.step = readFinish
		}
	}
	if sup.step == readFinish {
		return 0, io.EOF
	}
	return 0, errors.New("met error")
}

func NewStreamFileUploadBody(fileParamName string, filename string, fieldParams map[string]string) (body io.ReadCloser, contentType string, err error) {
	up := &streamUpload{}
	contentType, err = up.addFile(fileParamName, filename, fieldParams)
	if err != nil {
		return nil, "", err
	}
	return up, contentType, nil
}

func NewFileUploadRequest(uri string, fieldParams map[string]string, fileParamName, filename string) (*http.Request, error) {
	body, contextType, err := NewStreamFileUploadBody(fileParamName, filename, fieldParams)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", uri, body)
	if err != nil {
		body.Close()
		return nil, err
	}
	req.Header.Set("Content-Type", contextType)
	return req, nil
}
