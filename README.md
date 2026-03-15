
# AstrBot Windows 监控与控制插件项目 (PC Monitor & Control)

## 1. 项目背景与架构说明
本项目旨在为运行在 Docker 容器内的 `AstrBot` 机器人开发一个插件，使其能够监控并控制宿主机（Windows 操作系统）的硬件状态。
由于 Docker 的沙盒隔离特性，容器内的插件无法直接访问宿主机的 Windows API。因此，本项目采用 **C/S (客户端-服务端) 架构**：
- **服务端 (Windows Agent)**：运行在 Windows 宿主机本地，调用 Windows API 获取状态或执行控制命令，暴露为 HTTP RESTful API。
- **客户端 (AstrBot Plugin)**：运行在 Docker 容器内，作为 AstrBot 的插件，接收用户聊天指令，通过 `http://host.docker.internal` 访问宿主机 API，并返回结果给用户。

## 2. 目录结构
当前为单体仓库（Monorepo）结构，请严格按照官方最新模板规范，生成以下结构代码：
```text
PC_Monitor_Project/
├── windows_agent/                # Windows 本地服务端
│   ├── main.py                   # 服务端主程序
│   └── requirements.txt          # Python 依赖清单
└── astrbot_plugin_pcmonitor/     # AstrBot 插件端 (将打包为 zip)
    ├── main.py                   # 插件主逻辑代码
    └── metadata.yaml             # 插件元数据（注意：使用 yaml 格式）
```

## 3. 开发任务分解与技术栈要求

## 任务一：编写 Windows 本地服务端 (`windows_agent`)

- **技术栈**：推荐使用 `FastAPI` + `Uvicorn` 构建轻量级异步 API 服务。运行在本地 `0.0.0.0:5000` 端口。
- **核心功能与依赖库建议**：
  - 读取/设置亮度：`screen_brightness_control`
  - 读取/设置音量：`pycaw` 或 `ctypes` 调用 Windows Core Audio
  - 读取电量：`psutil`
  - 锁屏状态与控制：`ctypes` (如 `user32.LockWorkStation()`)
- **需要实现的 API 接口 (JSON 格式)**：
  1. `GET /status` -> 返回电量、亮度、音量。
  2. `POST /brightness` -> 接收 `{"value": 50}`，设置亮度为 50%。
  3. `POST /volume` -> 接收 `{"value": 50}`，设置系统音量为 50%。
  4. `POST /lock` -> 触发 Windows 锁屏。

## 任务二：编写 AstrBot 客户端插件 (`astrbot_plugin_pcmonitor`)

- **技术栈**：Python 异步编程，`aiohttp` (用于发 HTTP 请求)，AstrBot 最新插件标准 API。
- **网络通信核心要求**：请求宿主机 API 时，**必须使用 `http://host.docker.internal:5000` 作为基础 URL**，不可使用 `localhost` 或 `127.0.0.1`，否则无法穿透 Docker 网络。
- **需要实现的 AstrBot 交互指令**：
  - 注册插件类继承自 `Star`。
  - `@filter.command("电脑状态")`：请求 `/status` 并格式化输出。
  - `@filter.command("设置亮度")`：支持接收参数（如 `设置亮度 80`），请求 `/brightness`。
  - `@filter.command("设置音量")`：支持接收参数（如 `设置音量 20`），请求 `/volume`。
  - `@filter.command("电脑锁屏")`：请求 `/lock` 并返回执行结果。
- **文件 `metadata.yaml`**：请生成包含 name (`astrbot_plugin_pcmonitor`), author, version, description 的标准 YAML 文件。



