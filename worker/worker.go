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
	isReport        bool
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
	isReport bool,
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
		isReport:        isReport,

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

// getFilesInDirectory 获取目录中的所有文件名
func getFilesInDirectory(dirPath string) ([]string, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		if !entry.IsDir() {
			files = append(files, entry.Name())
		}
	}
	return files, nil
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

	// 生成并保存HTML报告
	if w.isReport {
		htmlReport := w.generateHTMLReport(resultDir)
		err = ioutil.WriteFile(resultDir+"report.html", []byte(htmlReport), 0644)
		if err != nil {
			log.Fatalf("生成HTML报告失败: %v", err)
		}
		fmt.Printf("HTML报告已保存至: %sreport.html\n", resultDir)
	}
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

// 新增HTML报告生成方法
func (w *Worker) generateHTMLReport(resultDir string) string {
	sb := strings.Builder{}
	sb.WriteString(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <title>样本回放测试报告</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; line-height: 1.6; }
        h1 { color: #2c3e50; border-bottom: 2px solid #3498db; padding-bottom: 10px; margin-bottom: 30px; }
        h2 { color: #3498db; margin-top: 30px; }
        .section { margin: 30px 0; padding: 20px; background-color: #f9f9f9; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        .stats-container { display: grid; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); gap: 20px; margin-top: 20px; }
        .stat-card { background-color: white; border-radius: 8px; padding: 15px; box-shadow: 0 2px 6px rgba(0,0,0,0.1); transition: transform 0.2s; }
        .stat-card:hover { transform: translateY(-5px); }
        .stat-title { font-size: 0.9em; color: #7f8c8d; margin-bottom: 5px; }
        .stat-value { font-size: 1.8em; font-weight: bold; color: #2c3e50; }
        .stat-value.high { color: #27ae60; }
        .stat-value.medium { color: #f39c12; }
        .stat-value.low { color: #e74c3c; }
        table { width: 100%; border-collapse: collapse; margin: 20px 0; box-shadow: 0 2px 5px rgba(0,0,0,0.1); }
        th, td { padding: 12px 15px; text-align: left; }
        th { background-color: #3498db; color: white; font-weight: bold; }
        tr:nth-child(even) { background-color: #f8f9fa; }
        tr:hover { background-color: #e9ecef; }
        .highlight-good { color: #27ae60; font-weight: bold; }
        .highlight-bad { color: #e74c3c; font-weight: bold; }
        .total-row { background-color: #e3f2fd; font-weight: bold; }
        .sample-list { padding-left: 20px; }
        .sample-category { margin: 20px 0; border: 1px solid #ddd; border-radius: 8px; overflow: hidden; }
        .category-header { background-color: #f1f1f1; padding: 10px 15px; cursor: pointer; display: flex; justify-content: space-between; align-items: center; }
        .category-header h3 { margin: 0; font-size: 1.1em; }
        .category-content { padding: 0 15px; max-height: 0; overflow: hidden; transition: max-height 0.3s ease-out, padding 0.3s ease-out; }
        .category-content.active { padding: 15px; max-height: 500px; overflow-y: auto; }
        .toggle-icon { transition: transform 0.3s ease; }
        .category-header.active .toggle-icon { transform: rotate(180deg); }
    </style>
</head>
<body>
`)
	sb.WriteString(fmt.Sprintf("    <h1>样本回放测试报告 - %s</h1>\n", time.Now().Format("2006-01-02 15:04:05")))

	// 添加统计概览
	sb.WriteString(`    <div class="section">
        <h2>统计概览</h2>
        <div class="stats-container">
`)
	// 总样本数量
	sb.WriteString(fmt.Sprintf(`            <div class="stat-card">
                <div class="stat-title">总样本数量</div>
                <div class="stat-value">%d</div>
            </div>
`, w.result.Total))
	// 成功数量
	sb.WriteString(fmt.Sprintf(`            <div class="stat-card">
                <div class="stat-title">成功数量</div>
                <div class="stat-value high">%d</div>
            </div>
`, w.result.Success))
	// 错误数量
	sb.WriteString(fmt.Sprintf(`            <div class="stat-card">
                <div class="stat-title">错误数量</div>
                <div class="stat-value low">%d</div>
            </div>
`, w.result.Error))
	// 检出率
	detectionRate := float64(w.result.TP) * 100 / float64(w.result.TP+w.result.FN)
	rateClass := "medium"
	if detectionRate >= 90 {
		rateClass = "high"
	} else if detectionRate <= 60 {
		rateClass = "low"
	}
	sb.WriteString(fmt.Sprintf(`            <div class="stat-card">
                <div class="stat-title">检出率</div>
                <div class="stat-value %s">%.2f%%</div>
            </div>
`, rateClass, detectionRate))
	// 误报率
	fpRate := float64(w.result.FP) * 100 / float64(w.result.TN+w.result.FP)
	fpClass := "high"
	if fpRate <= 5 {
		fpClass = "high"
	} else if fpRate <= 15 {
		fpClass = "medium"
	} else {
		fpClass = "low"
	}
	sb.WriteString(fmt.Sprintf(`            <div class="stat-card">
                <div class="stat-title">误报率</div>
                <div class="stat-value %s">%.2f%%</div>
            </div>
`, fpClass, fpRate))
	// 准确率
	accuracy := float64(w.result.TP+w.result.TN) * 100 / float64(w.result.TP+w.result.TN+w.result.FP+w.result.FN)
	accClass := "medium"
	if accuracy >= 90 {
		accClass = "high"
	} else if accuracy <= 70 {
		accClass = "low"
	}
	sb.WriteString(fmt.Sprintf(`            <div class="stat-card">
                <div class="stat-title">准确率</div>
                <div class="stat-value %s">%.2f%%</div>
            </div>
`, accClass, accuracy))
	// 平均耗时
	avgTime := float64(w.result.SuccessTimeCost) / float64(w.result.Success) / 1000000
	sb.WriteString(fmt.Sprintf(`            <div class="stat-card">
                <div class="stat-title">平均耗时</div>
                <div class="stat-value">%.2f毫秒</div>
            </div>
`, avgTime))
	sb.WriteString(`        </div>
    </div>
`)

	// 添加详细统计表格
	sb.WriteString(`    <div class="section">
        <h2>详细统计</h2>
        <table>
            <tr>
                <th>类别</th>
                <th>总数</th>
                <th>正确</th>
                <th>正确比例</th>
                <th>错误</th>
                <th>错误比例</th>
            </tr>
`)

	// 计算恶意样本统计数据
	totalMalicious := w.result.TP + w.result.FN
	maliciousCorrectRate := 0.0
	maliciousErrorRate := 0.0
	if totalMalicious > 0 {
		maliciousCorrectRate = float64(w.result.TP) / float64(totalMalicious) * 100
		maliciousErrorRate = float64(w.result.FN) / float64(totalMalicious) * 100
	}

	// 计算正常样本统计数据
	totalNormal := w.result.TN + w.result.FP
	normalCorrectRate := 0.0
	normalErrorRate := 0.0
	if totalNormal > 0 {
		normalCorrectRate = float64(w.result.TN) / float64(totalNormal) * 100
		normalErrorRate = float64(w.result.FP) / float64(totalNormal) * 100
	}

	// 计算总计数据
	totalSamples := totalMalicious + totalNormal
	totalCorrect := w.result.TP + w.result.TN
	totalError := w.result.FN + w.result.FP
	totalCorrectRate := 0.0
	totalErrorRate := 0.0
	if totalSamples > 0 {
		totalCorrectRate = float64(totalCorrect) / float64(totalSamples) * 100
		totalErrorRate = float64(totalError) / float64(totalSamples) * 100
	}

	// 添加恶意样本行
	sb.WriteString(fmt.Sprintf(`            <tr>
                <td>攻击样本</td>
                <td>%d</td>
                <td class="highlight-good">%d (拦截)</td>
                <td class="highlight-good">%.2f%%</td>
                <td class="highlight-bad">%d (漏报)</td>
                <td class="highlight-bad">%.2f%%</td>
            </tr>
`,
		totalMalicious, w.result.TP, maliciousCorrectRate, w.result.FN, maliciousErrorRate))

	// 添加正常样本行
	sb.WriteString(fmt.Sprintf(`            <tr>
                <td>白样本</td>
                <td>%d</td>
                <td class="highlight-good">%d (放行)</td>
                <td class="highlight-good">%.2f%%</td>
                <td class="highlight-bad">%d (误报)</td>
                <td class="highlight-bad">%.2f%%</td>
            </tr>
`,
		totalNormal, w.result.TN, normalCorrectRate, w.result.FP, normalErrorRate))

	// 添加总计行
	sb.WriteString(fmt.Sprintf(`            <tr class="total-row">
                <td>总计</td>
                <td>%d</td>
                <td>%d</td>
                <td>%.2f%%</td>
                <td>%d</td>
                <td>%.2f%%</td>
            </tr>
        </table>
    </div>
`,
		totalSamples, totalCorrect, totalCorrectRate, totalError, totalErrorRate))

	// 添加样本分类链接
	sb.WriteString(`    <div class="section">
        <h2>样本文件列表</h2>
        <p>点击分类标题可展开/折叠文件列表</p>
`)

	// 误拦截白样本
	sb.WriteString(`        <div class="sample-category">
            <div class="category-header" onclick="toggleCategory(this)">
                <h3>误拦截白样本</h3>
                <span class="toggle-icon">▼</span>
            </div>
            <div class="category-content">
`)
	whiteBlockDir := filepath.Join(resultDir, "white_block")
	whiteBlockFiles, err := getFilesInDirectory(whiteBlockDir)
	if err != nil {
		sb.WriteString(fmt.Sprintf("            <p>错误: 无法读取目录 %s: %v</p>\n", whiteBlockDir, err))
	} else if len(whiteBlockFiles) == 0 {
		sb.WriteString("            <p>无样本文件</p>\n")
	} else {
		sb.WriteString("            <ul class=\"sample-list\">\n")
		for _, file := range whiteBlockFiles {
			sb.WriteString(fmt.Sprintf("                <li><a href=\"white_block/%s\">%s</a></li>\n", file, file))
		}
		sb.WriteString("            </ul>\n")
	}
	sb.WriteString("            </div>\n        </div>\n")

	// 正确放行白样本
	sb.WriteString(`        <div class="sample-category">
            <div class="category-header" onclick="toggleCategory(this)">
                <h3>正确放行白样本</h3>
                <span class="toggle-icon">▼</span>
            </div>
            <div class="category-content">
`)
	whitePassDir := filepath.Join(resultDir, "white_pass")
	whitePassFiles, err := getFilesInDirectory(whitePassDir)
	if err != nil {
		sb.WriteString(fmt.Sprintf("            <p>错误: 无法读取目录 %s: %v</p>\n", whitePassDir, err))
	} else if len(whitePassFiles) == 0 {
		sb.WriteString("            <p>无样本文件</p>\n")
	} else {
		sb.WriteString("            <ul class=\"sample-list\">\n")
		for _, file := range whitePassFiles {
			sb.WriteString(fmt.Sprintf("                <li><a href=\"white_pass/%s\">%s</a></li>\n", file, file))
		}
		sb.WriteString("            </ul>\n")
	}
	sb.WriteString("            </div>\n        </div>\n")

	// 漏报攻击样本
	sb.WriteString(`        <div class="sample-category">
            <div class="category-header" onclick="toggleCategory(this)">
                <h3>漏报攻击样本</h3>
                <span class="toggle-icon">▼</span>
            </div>
            <div class="category-content">
`)
	attackPassDir := filepath.Join(resultDir, "attack_pass")
	attackPassFiles, err := getFilesInDirectory(attackPassDir)
	if err != nil {
		sb.WriteString(fmt.Sprintf("            <p>错误: 无法读取目录 %s: %v</p>\n", attackPassDir, err))
	} else if len(attackPassFiles) == 0 {
		sb.WriteString("            <p>无样本文件</p>\n")
	} else {
		sb.WriteString("            <ul class=\"sample-list\">\n")
		for _, file := range attackPassFiles {
			sb.WriteString(fmt.Sprintf("                <li><a href=\"attack_pass/%s\">%s</a></li>\n", file, file))
		}
		sb.WriteString("            </ul>\n")
	}
	sb.WriteString("            </div>\n        </div>\n")

	// 拦截攻击样本
	sb.WriteString(`        <div class="sample-category">
            <div class="category-header" onclick="toggleCategory(this)">
                <h3>正确拦截攻击样本</h3>
                <span class="toggle-icon">▼</span>
            </div>
            <div class="category-content">
`)
	attackBlackDir := filepath.Join(resultDir, "attack_black")
	attackBlackFiles, err := getFilesInDirectory(attackBlackDir)
	if err != nil {
		sb.WriteString(fmt.Sprintf("            <p>错误: 无法读取目录 %s: %v</p>\n", attackBlackDir, err))
	} else if len(attackBlackFiles) == 0 {
		sb.WriteString("            <p>无样本文件</p>\n")
	} else {
		sb.WriteString("            <ul class=\"sample-list\">\n")
		for _, file := range attackBlackFiles {
			sb.WriteString(fmt.Sprintf("                <li><a href=\"attack_black/%s\">%s</a></li>\n", file, file))
		}
		sb.WriteString("            </ul>\n")
	}
	sb.WriteString("            </div>\n        </div>\n    </div>\n")

	sb.WriteString(`    <script>
		function toggleCategory(element) {
			const content = element.nextElementSibling;
			element.classList.toggle('active');
			content.classList.toggle('active');
		}
	</script>
`)
	sb.WriteString("        </div>\n")

	sb.WriteString(`    </div>
</body>
</html>`)

	return sb.String()
}
