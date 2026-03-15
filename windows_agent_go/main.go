package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	ole "github.com/go-ole/go-ole"
)

// ==================== 数据模型 ====================

type ValueModel struct {
	Value int `json:"value"`
}

type BoolModel struct {
	Value bool `json:"value"`
}

type StatusResponse struct {
	Battery    int  `json:"battery"`
	Brightness int  `json:"brightness"`
	Volume     int  `json:"volume"`
	IsLocked   bool `json:"is_locked"`
}

// ==================== Windows 常量（硬编码，避免外部依赖）====================

const (
	// PROCESS_QUERY_INFORMATION / PROCESS_VM_READ 在新版 Go syscall 包中已移除
	// 直接硬编码其数值，与 Windows SDK 定义一致
	PROCESS_QUERY_INFORMATION = 0x0400
	PROCESS_VM_READ           = 0x0010
)

// ==================== Windows DLL 加载 ====================

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	psapi    = syscall.NewLazyDLL("psapi.dll")

	procLockWorkStation          = user32.NewProc("LockWorkStation")
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procGetSystemPowerStatus     = kernel32.NewProc("GetSystemPowerStatus")
	procGetProcessImageFileNameW = psapi.NewProc("GetProcessImageFileNameW")
)

// ==================== 电量 ====================

// SYSTEM_POWER_STATUS 对应 kernel32 的同名结构体
type SYSTEM_POWER_STATUS struct {
	ACLineStatus        byte
	BatteryFlag         byte
	BatteryLifePercent  byte
	SystemStatusFlag    byte
	BatteryLifeTime     uint32
	BatteryFullLifeTime uint32
}

// getBatteryPercent 通过 kernel32 读取电池电量，台式机无电池时返回 -1
func getBatteryPercent() int {
	var status SYSTEM_POWER_STATUS
	ret, _, _ := procGetSystemPowerStatus.Call(uintptr(unsafe.Pointer(&status)))
	if ret == 0 {
		return -1
	}
	if status.BatteryLifePercent == 255 { // 255 = 未知 / 无电池
		return -1
	}
	return int(status.BatteryLifePercent)
}

// ==================== 锁屏检测 ====================

// isLocked 判断当前是否处于锁屏状态
// 原理：检测前台窗口对应的进程名是否为 LogonUI.exe 或 LockApp.exe
func isLocked() bool {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return true
	}
	var pid uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		return false
	}

	// 直接使用硬编码的访问权限常量，不依赖 syscall.PROCESS_VM_READ
	handle, err := syscall.OpenProcess(PROCESS_QUERY_INFORMATION|PROCESS_VM_READ, false, pid)
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(handle)

	buf := make([]uint16, 260)
	ret, _, _ := procGetProcessImageFileNameW.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
	)
	if ret == 0 {
		return false
	}
	path := strings.ToLower(syscall.UTF16ToString(buf[:ret]))
	return strings.HasSuffix(path, "logonui.exe") || strings.HasSuffix(path, "lockapp.exe")
}

// ==================== 亮度（PowerShell + WMI）====================

// getBrightness 通过 PowerShell 调用 WMI 读取当前亮度
func getBrightness() int {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command",
		`(Get-WmiObject -Namespace root/WMI -Class WmiMonitorBrightness).CurrentBrightness`)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		return -1
	}
	val, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return -1
	}
	return val
}

// setBrightness 通过 PowerShell 调用 WMI 设置亮度
func setBrightness(value int) error {
	script := fmt.Sprintf(
		`(Get-WmiObject -Namespace root/WMI -Class WmiMonitorBrightnessMethods).WmiSetBrightness(1, %d)`,
		value,
	)
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Run()
}

// ==================== 音量（go-ole + COM vtable 直接调用）====================
//
// IAudioEndpointVolume vtable 布局（64位，每个函数指针8字节）：
//   偏移 0: IUnknown::QueryInterface
//   偏移 1: IUnknown::AddRef
//   偏移 2: IUnknown::Release
//   偏移 3: RegisterControlChangeNotify
//   偏移 4: UnregisterControlChangeNotify
//   偏移 5: GetChannelCount
//   偏移 6: SetMasterVolumeLevel
//   偏移 7: SetMasterVolumeLevelScalar  ← 设置音量
//   偏移 8: GetMasterVolumeLevel
//   偏移 9: GetMasterVolumeLevelScalar  ← 读取音量
//   偏移10: SetChannelVolumeLevel
//   偏移11: SetChannelVolumeLevelScalar
//   偏移12: GetChannelVolumeLevel
//   偏移13: GetChannelVolumeLevelScalar
//   偏移14: SetMute                     ← 静音控制
//   偏移15: GetMute

var (
	CLSID_MMDeviceEnumerator = ole.NewGUID("{BCDE0395-E52F-467C-8E3D-C4579291692E}")
	IID_IMMDeviceEnumerator  = ole.NewGUID("{A95664D2-9614-4F35-A746-DE8DB63617E6}")
	IID_IAudioEndpointVolume = ole.NewGUID("{5CDF2C82-841E-4546-9722-0CF74078229A}")
)

// vtableCall 通过 vtable 偏移 index 调用 COM 方法。
//
// 修复要点：
//   - syscall.Syscall9 是非可变参数函数，必须传入固定的9个参数，不足补0
//   - 不能使用 append(...) 展开切片传入，必须逐个显式列出
func vtableCall(obj uintptr, index uintptr, a1, a2, a3, a4, a5, a6 uintptr) (uintptr, error) {
	// 从 obj 的第一个字段读出 vtable 指针，再按偏移取函数地址
	vtbl := *(*uintptr)(unsafe.Pointer(obj))
	proc := *(*uintptr)(unsafe.Pointer(vtbl + index*8))

	// 使用 Syscall9，固定传9个参数（self + a1..a6 + 两个补零）
	r1, _, _ := syscall.Syscall9(
		proc,
		7, // 实际使用的参数个数：obj + a1..a6
		obj, a1, a2, a3, a4, a5, a6,
		0, 0, // 补齐到9个
	)

	// COM HRESULT：最高位为1（有符号负数）表示失败
	if int32(r1) < 0 {
		return r1, fmt.Errorf("COM HRESULT 错误: 0x%08X", uint32(r1))
	}
	return r1, nil
}

// comRelease 调用 IUnknown::Release（vtable 偏移2）
func comRelease(obj uintptr) {
	if obj != 0 {
		vtableCall(obj, 2, 0, 0, 0, 0, 0, 0)
	}
}

// getAudioEndpointVolume 返回 IAudioEndpointVolume 接口指针，调用方负责 comRelease
func getAudioEndpointVolume() (uintptr, error) {
	// 修复：go-ole 库中创建 COM 实例的正确函数是 ole.CreateInstance
	// ole.CoCreateInstance 在当前版本中不存在，是旧版接口名称
	unknown, err := ole.CreateInstance(CLSID_MMDeviceEnumerator, IID_IMMDeviceEnumerator)
	if err != nil {
		return 0, fmt.Errorf("创建 IMMDeviceEnumerator 失败: %v", err)
	}
	defer unknown.Release()

	enumPtr := uintptr(unsafe.Pointer(unknown))

	// 调用 IMMDeviceEnumerator::GetDefaultAudioEndpoint(eRender=0, eMultimedia=1)
	// IMMDeviceEnumerator 的 vtable 偏移应为 4
	var devicePtr uintptr
	if _, err = vtableCall(enumPtr, 4,
		0, // eRender
		1, // eMultimedia
		uintptr(unsafe.Pointer(&devicePtr)),
		0, 0, 0,
	); err != nil || devicePtr == 0 {
		return 0, fmt.Errorf("GetDefaultAudioEndpoint 失败: %v", err)
	}
	defer comRelease(devicePtr)

	// 调用 IMMDevice::Activate(iid, CLSCTX_ALL=23, NULL, &volumePtr)
	// IMMDevice 的 vtable 偏移同样是3
	var volumePtr uintptr
	if _, err = vtableCall(devicePtr, 3,
		uintptr(unsafe.Pointer(IID_IAudioEndpointVolume)),
		23, // CLSCTX_ALL
		0,  // pActivationParams = NULL
		uintptr(unsafe.Pointer(&volumePtr)),
		0, 0,
	); err != nil || volumePtr == 0 {
		return 0, fmt.Errorf("IMMDevice::Activate 失败: %v", err)
	}
	return volumePtr, nil
}

// getVolume 读取当前系统主音量（返回 0-100）
func getVolume() int {
	ole.CoInitialize(0)
	defer ole.CoUninitialize()

	volPtr, err := getAudioEndpointVolume()
	if err != nil {
		logMsg("读取音量失败: " + err.Error())
		return -1
	}
	defer comRelease(volPtr)

	// GetMasterVolumeLevelScalar：结果写入 float32 指针
	var scalar float32
	vtableCall(volPtr, 9, uintptr(unsafe.Pointer(&scalar)), 0, 0, 0, 0, 0)
	return int(scalar * 100)
}

// setVolume 设置主音量并强制解除静音
func setVolume(value int) error {
	ole.CoInitialize(0)
	defer ole.CoUninitialize()

	volPtr, err := getAudioEndpointVolume()
	if err != nil {
		return err
	}
	defer comRelease(volPtr)

	scalar := float32(value) / 100.0
	// float32 必须以其 IEEE754 位模式（uint32 → uintptr）传入 syscall
	scalarBits := uintptr(*(*uint32)(unsafe.Pointer(&scalar)))

	// SetMasterVolumeLevelScalar(scalar, pguidEventContext=NULL)
	vtableCall(volPtr, 7, scalarBits, 0, 0, 0, 0, 0)
	// SetMute(FALSE=0, pguidEventContext=NULL) — 解除静音
	vtableCall(volPtr, 14, 0, 0, 0, 0, 0, 0)
	return nil
}

// setMute 设置或解除静音
func setMute(mute bool) error {
	ole.CoInitialize(0)
	defer ole.CoUninitialize()

	volPtr, err := getAudioEndpointVolume()
	if err != nil {
		return err
	}
	defer comRelease(volPtr)

	muteVal := uintptr(0)
	if mute {
		muteVal = 1
	}
	vtableCall(volPtr, 14, muteVal, 0, 0, 0, 0, 0)
	return nil
}

// ==================== 日志 ====================

func logMsg(msg string) {
	log.Printf("💻 %s", msg)
}

// ==================== HTTP 工具 ====================

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"detail": msg})
}

// ==================== 路由处理器 ====================

// GET /status
func handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "仅支持 GET")
		return
	}
	logMsg("收到指令 ➡️ 获取电脑综合状态")

	battery := getBatteryPercent()
	brightness := getBrightness()
	volume := getVolume()
	locked := isLocked()

	logMsg(fmt.Sprintf("状态返回 ⬅️ 电量:%d%% | 亮度:%d%% | 音量:%d%% | 锁屏:%v",
		battery, brightness, volume, locked))
	writeJSON(w, http.StatusOK, StatusResponse{
		Battery:    battery,
		Brightness: brightness,
		Volume:     volume,
		IsLocked:   locked,
	})
}

// POST /brightness
func handleSetBrightness(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "仅支持 POST")
		return
	}
	var body ValueModel
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "请求体解析失败: "+err.Error())
		return
	}
	logMsg(fmt.Sprintf("收到指令 ➡️ 设置亮度为 %d%%", body.Value))
	if body.Value < 0 || body.Value > 100 {
		writeError(w, http.StatusBadRequest, "亮度值需在 0-100 之间")
		return
	}
	if err := setBrightness(body.Value); err != nil {
		logMsg("执行失败 ❌ " + err.Error())
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	logMsg("执行成功 ✅")
	writeJSON(w, http.StatusOK, map[string]any{"status": "success", "brightness": body.Value})
}

// POST /volume
func handleSetVolume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "仅支持 POST")
		return
	}
	var body ValueModel
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "请求体解析失败: "+err.Error())
		return
	}
	logMsg(fmt.Sprintf("收到指令 ➡️ 设置音量为 %d%%", body.Value))
	if body.Value < 0 || body.Value > 100 {
		writeError(w, http.StatusBadRequest, "音量值需在 0-100 之间")
		return
	}
	if err := setVolume(body.Value); err != nil {
		logMsg("执行失败 ❌ " + err.Error())
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	logMsg("执行成功 ✅")
	writeJSON(w, http.StatusOK, map[string]any{"status": "success", "volume": body.Value})
}

// POST /mute
func handleSetMute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "仅支持 POST")
		return
	}
	var body BoolModel
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "请求体解析失败: "+err.Error())
		return
	}
	stateStr := "解除静音"
	if body.Value {
		stateStr = "静音"
	}
	logMsg("收到指令 ➡️ 设置电脑状态为 " + stateStr)
	if err := setMute(body.Value); err != nil {
		logMsg("执行失败 ❌ " + err.Error())
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	logMsg("执行成功 ✅")
	writeJSON(w, http.StatusOK, map[string]any{"status": "success", "mute": body.Value})
}

// POST /lock
func handleLock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "仅支持 POST")
		return
	}
	logMsg("收到指令 ➡️ 执行电脑锁屏")
	ret, _, err := procLockWorkStation.Call()
	if ret == 0 {
		logMsg("执行失败 ❌ " + err.Error())
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	logMsg("执行成功 ✅")
	writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

// ==================== 主函数 ====================

func main() {
	log.SetFlags(log.Ltime)

	mux := http.NewServeMux()
	mux.HandleFunc("/status", handleStatus)
	mux.HandleFunc("/brightness", handleSetBrightness)
	mux.HandleFunc("/volume", handleSetVolume)
	mux.HandleFunc("/mute", handleSetMute)
	mux.HandleFunc("/lock", handleLock)

	addr := "0.0.0.0:5000"
	fmt.Println()
	fmt.Println("==========================================")
	fmt.Println("🚀 Windows 物理探针 (Go版) 已上线！")
	fmt.Printf("📡 正在监听: %s\n", addr)
	fmt.Println("==========================================")
	fmt.Println()

	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}