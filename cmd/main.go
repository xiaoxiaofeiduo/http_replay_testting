package main

import (
	"fmt"
	"http_replay_testting/utils"
	"http_replay_testting/worker"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/schollz/progressbar/v3"
)

func main() {
	Init()
	var addr string
	var isHttps bool

	if strings.HasPrefix(target, "http") {
		u, _ := url.Parse(target)
		if u.Scheme == "https" {
			isHttps = true
		}
		addr = u.Host
	}

	isWaf, blockStatusCode, err := utils.GetWafBlockStatusCode(target, mHost)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if !isWaf {
		fmt.Println("目标网站未开启waf")
		os.Exit(1)
	}

	fileList := make([]string, 0)
	if originalPayloadFile != "" {
		globFiles, err := utils.GetAllFiles(originalPayloadFile)
		if err != nil {
			fmt.Printf("open %s error: %s\n", originalPayloadFile, err)
			return
		}
		fileList = globFiles
	} else {
		fmt.Println("No test case directory")
		return
	}

	if len(fileList) == 0 {
		fmt.Println("no test case found")
		return
	}
	// progress bar
	progressBar := progressbar.NewOptions64(
		int64(len(fileList)),
		progressbar.OptionSetDescription("sending"),
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionSetWidth(10),
		progressbar.OptionThrottle(65*time.Millisecond),
		progressbar.OptionShowCount(),
		progressbar.OptionShowIts(),
		progressbar.OptionOnCompletion(func() {
			fmt.Fprint(os.Stderr, "\n")
		}),
		progressbar.OptionSpinnerType(14),
		progressbar.OptionFullWidth(),
		progressbar.OptionSetRenderBlankState(true),
		progressbar.OptionUseANSICodes(true),
	)

	worker := worker.NewWorker(
		addr,
		isHttps,
		fileList,
		blockStatusCode,
		isReport,
		worker.WithConcurrence(concurrent),
		worker.WithReqHost(mHost),
		worker.WithReqPerSession(requestPerSession),
		worker.WithTimeout(timeOut),
		worker.WithUseEmbedFS(originalPayloadFile == ""), // use embed test case fs when glob is empty
		worker.WithProgressBar(progressBar),
	)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		worker.Stop()
	}()
	worker.Run()
}
