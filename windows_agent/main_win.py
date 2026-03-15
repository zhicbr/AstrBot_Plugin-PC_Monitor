import ctypes
import time
from typing import Optional

import psutil
import screen_brightness_control as sbc
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel

import comtypes
from ctypes import cast, POINTER
from pycaw.pycaw import IAudioEndpointVolume, IMMDeviceEnumerator
from comtypes import GUID
CLSID_MMDeviceEnumerator = GUID("{BCDE0395-E52F-467C-8E3D-C4579291692E}")

app = FastAPI(title="PC Monitor & Control Agent")

class ValueModel(BaseModel):
    value: int
class BoolModel(BaseModel):
    value: bool
def log(msg: str):
    """自定义漂亮的时间戳日志"""
    print(f"[{time.strftime('%H:%M:%S')}] 💻 {msg}")

def get_audio_interface():
    """彻底绕过 pycaw 的壳，使用底层 COM 直接获取扬声器"""
    enumerator = comtypes.CoCreateInstance(
        CLSID_MMDeviceEnumerator,
        IMMDeviceEnumerator,
        comtypes.CLSCTX_INPROC_SERVER
    )
    # 0=eRender (渲染设备), 1=eMultimedia (多媒体)
    endpoint = enumerator.GetDefaultAudioEndpoint(0, 1)
    interface = endpoint.Activate(IAudioEndpointVolume._iid_, comtypes.CLSCTX_ALL, None)
    return cast(interface, POINTER(IAudioEndpointVolume))

def get_lock_status() -> bool:
    """通过检测前台窗口进程，精准判断是否处于锁屏界面"""
    try:
        user32 = ctypes.windll.user32
        hwnd = user32.GetForegroundWindow()
        if hwnd == 0:
            return True # 没有前台窗口通常意味着正在切换锁屏
        
        pid = ctypes.c_ulong()
        user32.GetWindowThreadProcessId(hwnd, ctypes.byref(pid))
        process = psutil.Process(pid.value)
        # LogonUI.exe 是密码输入界面, LockApp.exe 是Win10/11的锁屏壁纸界面
        if process.name().lower() in ["logonui.exe", "lockapp.exe"]:
            return True
        return False
    except Exception as e:
        log(f"读取锁屏状态失败: {e}")
        return False

@app.get("/status")
async def get_status():
    log("收到指令 ➡️ 获取电脑综合状态")
    
    # 1. 电池
    battery = psutil.sensors_battery()
    battery_percent = battery.percent if battery else -1

    # 2. 亮度
    try:
        brightness_list = sbc.get_brightness()
        brightness = brightness_list[0] if brightness_list else -1
    except Exception:
        brightness = -1

    # 3. 音量
    volume_level = -1
    try:
        comtypes.CoInitialize()
        volume_obj = get_audio_interface()
        volume_level = round(volume_obj.GetMasterVolumeLevelScalar() * 100)
    except Exception as e:
        log(f"读取音量失败: {e}")
    finally:
        comtypes.CoUninitialize()

    # 4. 锁屏
    is_locked = get_lock_status()

    log(f"状态返回 ⬅️ 电量:{battery_percent}% | 亮度:{brightness}% | 音量:{volume_level}% | 锁屏:{is_locked}")
    return {
        "battery": battery_percent,
        "brightness": brightness,
        "volume": volume_level,
        "is_locked": is_locked
    }

@app.post("/brightness")
async def set_brightness(data: ValueModel):
    log(f"收到指令 ➡️ 设置亮度为 {data.value}%")
    if not (0 <= data.value <= 100):
        raise HTTPException(status_code=400, detail="亮度值需在0-100之间")
    try:
        sbc.set_brightness(data.value)
        log("执行成功 ✅")
        return {"status": "success", "brightness": data.value}
    except Exception as e:
        log(f"执行失败 ❌ {e}")
        raise HTTPException(status_code=500, detail=str(e))

@app.post("/volume")
async def set_volume(data: ValueModel):
    log(f"收到指令 ➡️ 设置音量为 {data.value}%")
    if not (0 <= data.value <= 100):
        raise HTTPException(status_code=400, detail="音量值需在0-100之间")
    try:
        comtypes.CoInitialize()
        volume_obj = get_audio_interface()
        
        # 1. 调整音量数值
        volume_obj.SetMasterVolumeLevelScalar(data.value / 100.0, None)
        
        # 2. 强制解除静音状态 (0代表False，解除静音)
        volume_obj.SetMute(0, None)
        
        log("执行成功 ✅")
        return {"status": "success", "volume": data.value}
    except Exception as e:
        log(f"执行失败 ❌ {e}")
        raise HTTPException(status_code=500, detail=str(e))
    finally:
        comtypes.CoUninitialize()

@app.post("/lock")
async def trigger_lock():
    log("收到指令 ➡️ 执行电脑锁屏")
    try:
        ctypes.windll.user32.LockWorkStation()
        log("执行成功 ✅")
        return {"status": "success"}
    except Exception as e:
        log(f"执行失败 ❌ {e}")
        raise HTTPException(status_code=500, detail=str(e))

@app.post("/mute")
async def set_mute_state(data: BoolModel):
    state_str = "静音" if data.value else "解除静音"
    log(f"收到指令 ➡️ 设置电脑状态为 {state_str}")
    try:
        comtypes.CoInitialize()
        volume_obj = get_audio_interface()
        # 1 代表开启静音，0 代表解除静音
        volume_obj.SetMute(1 if data.value else 0, None)
        log("执行成功 ✅")
        return {"status": "success", "mute": data.value}
    except Exception as e:
        log(f"执行失败 ❌ {e}")
        raise HTTPException(status_code=500, detail=str(e))
    finally:
        comtypes.CoUninitialize()

if __name__ == "__main__":
    import uvicorn
    print("\n==========================================")
    print("🚀 AstrBot Windows 物理探针已上线！")
    print("📡 正在监听端口: 5000 ...")
    print("==========================================\n")
    # access_log=False 关掉 uvicorn 默认的刷屏日志，只看我们自己的！
    uvicorn.run(app, host="0.0.0.0", port=5000, access_log=False)