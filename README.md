# HTTP Replay Testing 项目说明
## 一、项目概述
http_replay_testting 项目主要用于对WAF设备的请求进行回放测试，通过对WAF设备的请求进行回放测试，可以验证WAF的防护效果。

## 二、项目结构
http_replay_testting 项目主要包括以下几个部分：
```
http_replay_testting/
├── .git/
├── .gitignore
├── build.sh
├── cmd/
│   ├── flags.go
│   └── main.go
├── go.mod
├── http/
│   ├── .gitignore
│   ├── Makefile
│   ├── common.rl
│   ├── connect.go
│   ├── request.go
│   ├── request_parser.go
│   ├── request_parser.rl
│   ├── request_test.go
│   ├── response.go
│   ├── response_parser.go
│   ├── response_parser.rl
│   ├── response_test.go
│   └── struct.go
├── template/
│   ├── http.black
│   └── http.white
├── utils/
│   └── utils.go
├── worker/
│   └── worker.go
```

## 三、项目使用
### 1. 编译项目

```
./build.sh
```
### 2. 运行项目

```
./build/http_replay_testting -t http://10.10.121.18 -p payload_file
```
`-t` 为目标地址，`-p` 为payload文件路径。

### 3. payload文件格式
payload文件格式可参考`template`目录下的`http.black`（黑样本以black为文件后缀）和`http.white`（非black文件后缀的都为白样本）文件。

### 4. 报告结果
测试结果会汇聚到`result`目录下，报告文件为`report.html`。


## 四、项目参考（基于下面的开源项目增加了结果汇聚功能）

[blazehttp](https://github.com/chaitin/blazehttp)