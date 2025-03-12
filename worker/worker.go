package worker

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	replayhttp "http_replay_testting/http"
)

type Progress interface {
	Add(n int) error
}

type Worker struct {
	ctx    context.Context
	cancel context.CancelFunc

	concurrence   int // concurrent connections
	fileList      []string
	jobs          chan *Job
	jobResult     chan *Job
	jobResultDone chan struct{}
	result        *Result
	progressBar   Progress

	addr            string // target addr
	isHttps         bool   // is https
	timeout         int    // connection timeout
	blockStatusCode int    // block status code
	reqHost         string // request host of header
	reqPerSession   bool   // request per session
	useEmbedFS      bool
	resultCh        chan *Result
}

type WorkerOption func(*Worker)

func WithTimeout(timeout int) WorkerOption {
	return func(w *Worker) {
		w.timeout = timeout
	}
}

func WithReqHost(reqHost string) WorkerOption {
	return func(w *Worker) {
		w.reqHost = reqHost
	}
}

func WithReqPerSession(reqPerSession bool) WorkerOption {
	return func(w *Worker) {
		w.reqPerSession = reqPerSession
	}
}

func WithUseEmbedFS(useEmbedFS bool) WorkerOption {
	return func(w *Worker) {
		w.useEmbedFS = useEmbedFS
	}
}

func WithConcurrence(c int) WorkerOption {
	return func(w *Worker) {
		w.concurrence = c
	}
}

func WithResultCh(ch chan *Result) WorkerOption {
	return func(w *Worker) {
		w.resultCh = ch
	}
}

func WithProgressBar(pb Progress) WorkerOption {
	return func(w *Worker) {
		w.progressBar = pb
	}
}

func (w *Worker) Stop() {
	w.cancel()
}

func NewWorker(
	addr string,
	isHttps bool,
	fileList []string,
	blockStatusCode int,
	options ...WorkerOption,
) *Worker {
	w := &Worker{
		concurrence: 10, // default 10

		// payloads
		fileList: fileList,

		// connect target & config
		addr:            addr,
		isHttps:         isHttps,
		timeout:         1000, // 1000ms
		blockStatusCode: blockStatusCode,

		jobs:          make(chan *Job),
		jobResult:     make(chan *Job),
		jobResultDone: make(chan struct{}),

		result: &Result{
			Total: int64(len(fileList)),
		},
	}
	w.ctx, w.cancel = context.WithCancel(context.Background())

	for _, opt := range options {
		opt(w)
	}
	return w
}

type Job struct {
	FilePath string
	Result   *JobResult
}

type JobResult struct {
	IsWhite    bool
	IsPass     bool
	Success    bool
	TimeCost   int64
	StatusCode int
	Err        string
}

type Result struct {
	Total           int64 // total poc
	Error           int64
	Success         int64 // success poc
	SuccessTimeCost int64 // success success cost
	TN              int64
	FN              int64
	TP              int64
	FP              int64
	Job             *Job
}

type Output struct {
	Out string
	Err string
}

/*样本统计代码*/
// 声明加锁变量
var (
	whiteBlock = []string{}
	whiteMutex sync.Mutex // 全局互斥锁
)

var (
	whitePass      = []string{}
	whitePassMutex sync.Mutex // 全局互斥锁
)

var (
	attackPass  = []string{}
	attackMutex sync.Mutex // 全局互斥锁
)

var (
	attackBlack      = []string{}
	attackBlackMutex sync.Mutex // 全局互斥锁
)

func writeToFile(filename string, list []string) error {
	// 将列表中的元素连接成字符串
	content := []byte("")
	for _, item := range list {
		content = append(content, []byte(item+"\n")...)
	}

	// 将内容写入文件
	err := ioutil.WriteFile(filename, content, 0644)
	if err != nil {
		return err
	}

	return nil
}

/*样本统计代码*/

/*样本归档代码*/

// clearOrCreateDirectory 清空一个目录，如果目录不存在就创建
func clearOrCreateDirectory(dirPath string) error {
	// 检查目录是否存在
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		// 目录不存在，创建目录
		return os.MkdirAll(dirPath, 0755)
	}

	// 目录存在，清空目录
	dir, err := os.Open(dirPath)
	if err != nil {
		return err
	}
	defer dir.Close()

	names, err := dir.Readdirnames(-1)
	if err != nil {
		return err
	}

	for _, name := range names {
		err = os.RemoveAll(filepath.Join(dirPath, name))
		if err != nil {
			return err
		}
	}

	return nil
}

// copyFile 复制单个文件从 src 到 dst。
// 如果 dst 文件已存在，则会被覆盖。
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// copyFilesFromList 按行读取 fileListPath 文件中的文件路径，并将文件复制到 targetDir。
// 如果 targetDir 不存在，则创建该目录。
func copyFilesFromList(fileListPath, targetDir string) error {
	// 清空目录或创建目录
	if err := clearOrCreateDirectory(targetDir); err != nil {
		return err
	}

	// 打开文件列表文件
	fileList, err := os.Open(fileListPath)
	if err != nil {
		return err
	}
	defer fileList.Close()

	// 创建一个新的bufio.Scanner用于读取文件
	scanner := bufio.NewScanner(fileList)
	for scanner.Scan() {
		line := scanner.Text()
		line = filepath.Clean(line) // 清除非法字符

		// 获取文件的绝对路径
		sourcePath, err := filepath.Abs(line)
		if err != nil {
			fmt.Printf("Failed to get absolute path for %s: %v\n", line, err)
			continue
		}

		// 目标文件路径
		targetPath := filepath.Join(targetDir, filepath.Base(sourcePath))

		// 复制文件
		if err := copyFile(sourcePath, targetPath); err != nil {
			fmt.Printf("Failed to copy file %s to %s: %v\n", sourcePath, targetPath, err)
			continue
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading file list: %v", err)
	}
	return nil
}

/*样本归档代码*/

func (w *Worker) Run() {
	go func() {
		w.jobProducer()
	}()

	go func() {
		w.processJobResult()
		w.jobResultDone <- struct{}{}
	}()

	wg := sync.WaitGroup{}

	for i := 0; i < w.concurrence; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.runWorker()
		}()
	}
	wg.Wait()

	close(w.jobResult)
	<-w.jobResultDone
	resultDir := "result/"

	//创建归档目录
	err := clearOrCreateDirectory(resultDir)
	if err != nil {
		log.Fatal(err)
	}
	/*样本统计代码*/
	//将误拦截的白样本记录到white_block.txt中
	err = writeToFile(resultDir+"white_block.txt", whiteBlock)
	if err != nil {
		log.Fatal(err)
	}

	//将放过的白样本记录到white_pass.txt中
	err = writeToFile(resultDir+"white_pass.txt", whitePass)
	if err != nil {
		log.Fatal(err)
	}

	//将漏检测的黑样本记录到attack_pass.txt中
	err = writeToFile(resultDir+"attack_pass.txt", attackPass)
	if err != nil {
		log.Fatal(err)
	}

	//将检测到的黑样本记录到attack_black.txt中
	err = writeToFile(resultDir+"attack_black.txt", attackBlack)
	if err != nil {
		log.Fatal(err)
	}

	/*样本统计代码*/

	/*样本归档代码*/
	err = copyFilesFromList(resultDir+"white_block.txt", resultDir+"white_block")
	if err != nil {
		log.Fatal(err)
	}

	err = copyFilesFromList(resultDir+"white_pass.txt", resultDir+"white_pass")
	if err != nil {
		log.Fatal(err)
	}

	err = copyFilesFromList(resultDir+"attack_pass.txt", resultDir+"attack_pass")
	if err != nil {
		log.Fatal(err)
	}

	err = copyFilesFromList(resultDir+"attack_black.txt", resultDir+"attack_black")
	if err != nil {
		log.Fatal(err)
	}

	/*样本归档代码*/
	fmt.Println(w.generateResult())
}

func (w *Worker) runWorker() {
	for job := range w.jobs {
		func() {
			defer func() {
				w.jobResult <- job
			}()
			filePath := job.FilePath
			req := new(replayhttp.Request)

			if err := req.ReadFile(filePath); err != nil {
				job.Result.Err = fmt.Sprintf("read request file: %s error: %s\n", filePath, err)
				return
			}

			if w.reqHost != "" {
				req.SetHost(w.reqHost)
			} else {
				req.SetHost(w.addr)
			}

			if w.reqPerSession {
				// one http request one connection
				req.SetHeader("Connection", "close")
			}

			req.CalculateContentLength()

			start := time.Now()
			conn := replayhttp.Connect(w.addr, w.isHttps, w.timeout)
			if conn == nil {
				job.Result.Err = fmt.Sprintf("connect to %s failed!\n", w.addr)
				return
			}
			nWrite, err := req.WriteTo(*conn)
			if err != nil {
				job.Result.Err = fmt.Sprintf("send request poc: %s length: %d error: %s", filePath, nWrite, err)
				return
			}

			rsp := new(replayhttp.Response)
			if err = rsp.ReadConn(*conn); err != nil {
				job.Result.Err = fmt.Sprintf("read poc file: %s response, error: %s", filePath, err)
				return
			}
			elap := time.Since(start).Nanoseconds()
			(*conn).Close()
			job.Result.Success = true
			if strings.HasSuffix(job.FilePath, "white") {
				job.Result.IsWhite = true // white case
			}

			code := rsp.GetStatusCode()
			job.Result.StatusCode = code
			if code != w.blockStatusCode {
				job.Result.IsPass = true
			}
			job.Result.TimeCost = elap
		}()
	}
}

func (w *Worker) processJobResult() {
	for job := range w.jobResult {
		if job.Result.Success {
			w.result.Success++
			w.result.SuccessTimeCost += job.Result.TimeCost
			if job.Result.IsWhite {
				if job.Result.IsPass {
					w.result.TN++
					/*样本统计代码*/
					//将放过的白样本记录whitePass中
					whitePassMutex.Lock()
					whitePass = append(whitePass, job.FilePath)
					whitePassMutex.Unlock()
					/*样本统计代码*/
				} else {
					w.result.FP++
					/*样本统计代码*/
					//将误拦截的白样本记录whiteBlock中
					whiteMutex.Lock()
					whiteBlock = append(whiteBlock, job.FilePath)
					whiteMutex.Unlock()
					/*样本统计代码*/
				}
			} else {
				if job.Result.IsPass {
					w.result.FN++
					/*样本统计代码*/
					//将漏检测的黑样本记录attackPass中
					attackMutex.Lock()
					attackPass = append(attackPass, job.FilePath)
					attackMutex.Unlock()
					/*样本统计代码*/
				} else {
					w.result.TP++
					/*样本统计代码*/
					//将检测到的黑样本记录attackBLack中
					attackBlackMutex.Lock()
					attackBlack = append(attackBlack, job.FilePath)
					attackBlackMutex.Unlock()
					/*样本统计代码*/
				}
			}
		} else {
			w.result.Error++
		}
		if w.resultCh != nil {
			r := *w.result
			r.Job = job
			w.resultCh <- &r
		}
	}
}

func (w *Worker) jobProducer() {
	defer close(w.jobs)
	for _, f := range w.fileList {
		select {
		case <-w.ctx.Done():
			return
		default:
			w.jobs <- &Job{
				FilePath: f,
				Result:   &JobResult{},
			}
			if w.progressBar != nil {
				_ = w.progressBar.Add(1)
			}
		}
	}
}

func (w *Worker) generateResult() string {
	sb := strings.Builder{}
	sb.WriteString(fmt.Sprintf("总样本数量: %d    成功: %d    错误: %d\n", w.result.Total, w.result.Success, (w.result.Total - w.result.Success)))
	sb.WriteString(fmt.Sprintf("检出率: %.2f%% (恶意样本总数: %d , 正确拦截: %d , 漏报放行: %d)\n", float64(w.result.TP)*100/float64(w.result.TP+w.result.FN), w.result.TP+w.result.FN, w.result.TP, w.result.FN))
	sb.WriteString(fmt.Sprintf("误报率: %.2f%% (正常样本总数: %d , 正确放行: %d , 误报拦截: %d)\n", float64(w.result.FP)*100/float64(w.result.TN+w.result.FP), w.result.TN+w.result.FP, w.result.TN, w.result.FP))
	sb.WriteString(fmt.Sprintf("准确率: %.2f%% (正确拦截 + 正确放行）/样本总数 \n", float64(w.result.TP+w.result.TN)*100/float64(w.result.TP+w.result.TN+w.result.FP+w.result.FN)))
	sb.WriteString(fmt.Sprintf("平均耗时: %.2f毫秒\n", float64(w.result.SuccessTimeCost)/float64(w.result.Success)/1000000))
	return sb.String()
}
