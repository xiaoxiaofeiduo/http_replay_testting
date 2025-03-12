package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"
)

var (
	target              string // the target web site, example: http://192.168.0.1:8080
	originalPayloadFile string // use glob expression to select multi files,original Payload File
	timeOut             = 1000 // default 1000 ms
	concurrent          = 10   // default 10 concurrent workers
	mHost               string // modify host header
	requestPerSession   bool   // send request per session
)

func Init() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: ./http_replay -t <url> -p <payloads file>")
		os.Exit(1)
	}
	flag.StringVar(&target, "t", "", "target website, example: http://192.168.0.1:8080")
	flag.IntVar(&concurrent, "c", 10, "concurrent workers, default 10")
	flag.StringVar(&originalPayloadFile, "p", "", "glob expression, example: payloads/")
	flag.IntVar(&timeOut, "timeout", 1000, "connection timeout, default 1000 ms")
	flag.StringVar(&mHost, "H", "", "modify host header")
	flag.BoolVar(&requestPerSession, "rps", true, "send request per session")
	flag.Parse()
	if url, err := url.Parse(target); err != nil || url.Scheme == "" || url.Host == "" {
		fmt.Println("invalid target url, example: http://www.a.com")
		os.Exit(1)
	}
	if originalPayloadFile == "" {
		fmt.Println("No test case directory, example: payloads/")
		os.Exit(1)
	}

}
