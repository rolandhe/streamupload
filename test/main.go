package main

import (
	"fmt"
	"github.com/rolandhe/streamupload"
	"io"
	"net/http"
)

var client = &http.Client{}

func init() {
	streamupload.DebugMode = true
	streamupload.LoggerFunc = func(traceId string, message string) {
		fmt.Printf("traceI=%s,message=%s\n", traceId, message)
	}
}

func main() {
	params := map[string]string{
		"k1": "101",
		"k2": "333",
	}
	req, err := streamupload.NewFileUploadRequest("http://localhost:8080/upload", params, "file", "./test/php2java2.pdf", "jjjjjjj-trace-00093")
	if err != nil {
		fmt.Println(err)
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(resp.Status)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	fmt.Println(string(body))
}
