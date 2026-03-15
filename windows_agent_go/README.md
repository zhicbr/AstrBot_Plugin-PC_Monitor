> [!CAUTION]
>
> 该README记录让ai将python版代码改成Go版代码的提示词

# 提示词1

   ```
   # 任务:将 Python 写的 Windows 监控服务用 Go 语言重构

   ## 背景说明

   我需要一个运行在 Windows 本地的极轻量级 HTTP 服务。之前是用 Python + FastAPI 写的,但为了将内存占用压缩到 10MB 以内,并编译为单文件 `.exe`,我需要你将其重写为 Go 语言版本。

   ## 技术栈与约束要求

      1. **语言**:Go (Golang)。
      2. **Web 框架**:为了极致轻量,请直接使用 Go 标准库 `net/http`,不需要 Gin 等大型框架。监听端口为 `0.0.0.0:5000`。
      3. **返回格式**:所有接口均返回标准的 JSON 格式,与之前的 Python 版本保持一致。
      4. **Windows API 调用**:
         - **锁屏**:调用 `user32.dll` 的 `LockWorkStation`。
         - **音量与静音**:必须使用 `github.com/go-ole/go-ole` 库来调用 Windows Core Audio API (IMMDeviceEnumerator, IAudioEndpointVolume)。
         - **电量**:调用 `kernel32.dll` 的 `GetSystemPowerStatus`。
         - **亮度**:可以使用 WMI 调用,或者使用第三方库 `github.com/kbinani/screenshot` 相关的亮度控制,或者执行 PowerShell 脚本命令作为退路方案。

   ## 需要实现的接口清单

      1. `GET /status`:返回 `{"battery": 70, "brightness": 50, "volume": 20, "is_locked": false}`。
      2. `POST /brightness`:接收 `{"value": 50}`,设置亮度。
      3. `POST /volume`:接收 `{"value": 20}`,设置系统音量(设置音量的同时需解除静音状态)。
      4. `POST /mute`:接收 `{"value": true/false}`,设置静音或解除静音。
      5. `POST /lock`:执行系统锁屏。 请为我生成完整的 `main.go` 代码,并附带必要的 `go mod` 初始化和依赖获取命令(`go get`)。代码中请加上清晰的中文注释。
   
   ```

# 提示词2

```
我使用的是最新版本的 Go (1.26.1)，你在上一步提供的代码使用了过时的语法和库，导致编译失败。
以下是 `go build` 时的具体报错信息：
# command-line-arguments
.\main.go:96:79: undefined: syscall.PROCESS_VM_READ
.\main.go:200:18: cannot use ... in call to non-variadic syscall.Syscall
.\main.go:202:18: cannot use ... in call to non-variadic syscall.Syscall6
.\main.go:204:18: cannot use ... in call to non-variadic syscall.Syscall9
.\main.go:228:12: undefined: ole.CoCreateInstance
请根据最新版 Go 的规范，修复上述问题并重新生成完整的 `main.go` 代码。
修复重点提示：
1. 现代 Go 版本的 `syscall.Syscall` 不再支持可变参数（non-variadic），请老老实实地传递完整的参数列表（补齐 0）。
2. `syscall.PROCESS_VM_READ` 等 Windows 特有常量请改用 `golang.org/x/sys/windows` 库来获取，或者直接在代码里硬编码定义其常量值以减少外部依赖。
3. 检查 `go-ole` 库的方法名调用，比如 `ole.CoCreateInstance` 是否在当前库版本下有变化（或者是否有拼写遗漏）。
我需要一份能直接在最新版 Go 环境下 `go build` 成功的代码。
```

