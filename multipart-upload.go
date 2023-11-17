package streamupload

import (
	"bytes"
	"errors"
	"fmt"
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
)

var (
	DebugMode  bool
	LoggerFunc func(traceId string, message string)
)

type streamUpload struct {
	file     *os.File
	firstSeg bytes.Buffer
	lastSeg  bytes.Buffer
	step     stepEnum

	outLen     int
	outFileLen int

	traceId string
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
	w := multipart.NewWriter(&sup.firstSeg)
	_, err = w.CreateFormFile(fileParamName, filepath.Base(filename))
	if err != nil {
		sup.Close()
		return "", err
	}
	formDataContentType := w.FormDataContentType()
	boundary := w.Boundary()

	w = multipart.NewWriter(&sup.lastSeg)
	w.SetBoundary(boundary)
	if len(fieldParams) > 0 {
		sup.lastSeg.WriteString("\r\n")
	}
	for key, val := range fieldParams {
		err = w.WriteField(key, val)
		if err != nil {
			sup.Close()
			return "", err
		}
	}
	w.Close()
	if isDebug() {
		LoggerFunc(sup.traceId, fmt.Sprintf("last:%s", sup.lastSeg.String()))
	}
	return formDataContentType, err
}

func (sup *streamUpload) Read(p []byte) (n int, err error) {
	bufLen := len(p)
	if isDebug() {
		LoggerFunc(sup.traceId, fmt.Sprintf("read buff size:%d", bufLen))
	}
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
				if isDebug() {
					LoggerFunc(sup.traceId, fmt.Sprintf("read data size:%d,step %d", gotLen, sup.step))
				}
				sup.outLen += gotLen
				return gotLen, nil
			}
			sup.step = readFileContentStep
		} else {
			sup.step = readFileContentStep
		}
		if isDebug() {
			LoggerFunc(sup.traceId, fmt.Sprintf("buff is not full:%d,next to read file", gotLen))
		}
	}

	if gotLen > 0 {
		buff = p[gotLen:bufLen]
	}
	if sup.step == readFileContentStep {
		for {
			n, err = sup.file.Read(buff)
			if err != nil && err != io.EOF {
				return 0, err
			}
			if n > 0 {
				gotLen += n
				sup.outFileLen += n
				buff = p[gotLen:bufLen]
			}

			if err == io.EOF {
				if isDebug() {
					LoggerFunc(sup.traceId, fmt.Sprintf("finish read file,this time got len %d,and buff remain:%d", n, bufLen-gotLen))
				}
				sup.step = readLastSegStep
				break
			}
			if gotLen == bufLen {
				if isDebug() {
					LoggerFunc(sup.traceId, fmt.Sprintf("return data to http size:%d,step %d,this time read %d", gotLen, sup.step, n))
				}
				sup.outLen += gotLen
				return gotLen, err
			}
			if isDebug() {
				LoggerFunc(sup.traceId, fmt.Sprintf("got file data %d,and buff remain:%d", n, bufLen-gotLen))
			}
		}
	}

	if gotLen > 0 {
		buff = p[gotLen:bufLen]
	}

	if sup.step == readLastSegStep {
		n, err = sup.lastSeg.Read(buff)
		if err != nil && err != io.EOF {
			return n, err
		}
		if n > 0 {
			gotLen += n
		}
		if gotLen < bufLen {
			if isDebug() {
				LoggerFunc(sup.traceId, fmt.Sprintf("buff is not fill, finish all read, return http size:%d,this time read %d,step %d, body size:%d,file size:%d", gotLen, n, sup.step, sup.outLen, sup.outFileLen))
			}
			sup.outLen += gotLen
			return gotLen, io.EOF
		}
		if isDebug() {
			if err == io.EOF {
				LoggerFunc(sup.traceId, fmt.Sprintf("finish all read,return http size:%d,this time read %d,step %d,body size:%d,file size:%d", gotLen, n, sup.step, sup.outLen, sup.outFileLen))
			} else {
				LoggerFunc(sup.traceId, fmt.Sprintf("return data to http size:%d,this time read %d,step %d,err:%v", gotLen, n, sup.step, err))
			}
		}
		sup.outLen += gotLen
		return gotLen, err
	}
	if isDebug() {
		LoggerFunc(sup.traceId, "met error")
	}
	return 0, errors.New("met error")
}

func isDebug() bool {
	return DebugMode && LoggerFunc != nil
}

func NewStreamFileUploadBody(traceId, fileParamName string, filename string, fieldParams map[string]string) (body io.ReadCloser, contentType string, err error) {
	up := &streamUpload{
		traceId: traceId,
	}

	contentType, err = up.addFile(fileParamName, filename, fieldParams)
	if err != nil {
		return nil, "", err
	}
	return up, contentType, nil
}

func NewFileUploadRequest(uri string, fieldParams map[string]string, fileParamName, filename string, traceId string) (*http.Request, error) {
	body, contextType, err := NewStreamFileUploadBody(traceId, fileParamName, filename, fieldParams)
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
