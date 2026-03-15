import aiohttp
from astrbot.api.all import *
from astrbot.api.all import llm_tool
from astrbot.api.event import filter

BASE_URL = "http://host.docker.internal:5000"

@register("astrbot_plugin_pcmonitor", "astrbot_developer", "1.0.0", "PC Monitor Plugin")
class PCMonitorPlugin(Star):
    def __init__(self, context: Context):
        super().__init__(context)
    
    @filter.command("电脑状态")
    async def pc_status(self, event: AstrMessageEvent):
        """获取电脑当前的电量、亮度和音量。"""
        try:
            async with aiohttp.ClientSession() as session:
                async with session.get(f"{BASE_URL}/status", timeout=5) as resp:
                    if resp.status == 200:
                        data = await resp.json()
                        battery = data.get("battery", -1)
                        brightness = data.get("brightness", -1)
                        volume = data.get("volume", -1)
                        
                        msg = [
                            "🖥️ [当前电脑状态]",
                            f"🔋 电量: {battery}%" if battery != -1 else "🔋 电量: 获取失败",
                            f"☀️ 亮度: {brightness}%" if brightness != -1 else "☀️ 亮度: 获取失败",
                            f"🔊 音量: {volume}%" if volume != -1 else "🔊 音量: 获取失败"
                        ]
                        yield event.plain_result("\n".join(msg))
                    else:
                        yield event.plain_result(f"获取状态失败，HTTP状态码: {resp.status}")
        except Exception as e:
            yield event.plain_result(f"请求宿主机API发生异常: {str(e)}")

    @filter.command("设置亮度")
    async def pc_set_brightness(self, event: AstrMessageEvent, value: int):
        """设置电脑亮度 (0-100)。例如: 设置亮度 80"""
        if not (0 <= value <= 100):
            yield event.plain_result("亮度值必须在 0 到 100 之间。")
            return
            
        try:
            async with aiohttp.ClientSession() as session:
                payload = {"value": value}
                async with session.post(f"{BASE_URL}/brightness", json=payload, timeout=5) as resp:
                    if resp.status == 200:
                        yield event.plain_result(f"✅ 成功将亮度设置为 {value}%")
                    else:
                        text = await resp.text()
                        yield event.plain_result(f"❌ 设置亮度失败: {text}")
        except Exception as e:
            yield event.plain_result(f"请求宿主机API发生异常: {str(e)}")

    @filter.command("设置音量")
    async def pc_set_volume(self, event: AstrMessageEvent, value: int):
        """设置电脑系统音量 (0-100)。例如: 设置音量 20"""
        if not (0 <= value <= 100):
            yield event.plain_result("音量值必须在 0 到 100 之间。")
            return
            
        try:
            async with aiohttp.ClientSession() as session:
                payload = {"value": value}
                async with session.post(f"{BASE_URL}/volume", json=payload, timeout=5) as resp:
                    if resp.status == 200:
                        yield event.plain_result(f"✅ 成功将系统音量设置为 {value}%")
                    else:
                        text = await resp.text()
                        yield event.plain_result(f"❌ 设置音量失败: {text}")
        except Exception as e:
            yield event.plain_result(f"请求宿主机API发生异常: {str(e)}")

    @filter.command("电脑锁屏")
    async def pc_lock(self, event: AstrMessageEvent):
        """触发宿主机电脑锁屏"""
        try:
            async with aiohttp.ClientSession() as session:
                async with session.post(f"{BASE_URL}/lock", timeout=5) as resp:
                    if resp.status == 200:
                        yield event.plain_result("🔒 电脑已成功锁屏。")
                    else:
                        text = await resp.text()
                        yield event.plain_result(f"❌ 锁屏失败: {text}")
        except Exception as e:
            yield event.plain_result(f"请求宿主机API发生异常: {str(e)}")

    @llm_tool(name="get_pc_status", description="当用户想要获取或查询宿主机 Windows 电脑当前的系统状态（包括电池电量、屏幕亮度、系统音量和是否锁屏）时调用此工具。")
    async def get_pc_status(self, event: AstrMessageEvent):
        try:
            async with aiohttp.ClientSession() as session:
                async with session.get(f"{BASE_URL}/status", timeout=5) as resp:
                    if resp.status == 200:
                        data = await resp.json()
                        battery = data.get("battery", -1)
                        brightness = data.get("brightness", -1)
                        volume = data.get("volume", -1)
                        
                        # 新增：接住锁屏状态
                        is_locked = data.get("is_locked", False)
                        lock_status_text = "已锁屏" if is_locked else "未锁屏，处于使用状态"
                        
                        return f"当前系统状态 -> 电量: {battery}%, 亮度: {brightness}%, 音量: {volume}%, 锁屏状态: {lock_status_text}"
                    else:
                        return f"获取状态失败，HTTP状态码: {resp.status}"
        except Exception as e:
            return f"请求宿主机API发生异常: {str(e)}"

    @llm_tool(name="set_pc_brightness", description="设置宿主机 Windows 电脑屏幕亮度。")
    async def set_pc_brightness(self, event: AstrMessageEvent, brightness: int):
        """
        当用户想要设置、调整屏幕亮度时调用。
        Args:
            brightness (int): 目标亮度值(0-100)。注意：这是必填参数，必须提供一个具体的整数！如果用户只是说“调亮”或“调暗”，请你自行推算一个合适的数值（例如当前亮度减去20）并传入。
        """
        if not (0 <= brightness <= 100):
            return "亮度值必须在 0 到 100 之间。"
            
        try:
            async with aiohttp.ClientSession() as session:
                payload = {"value": brightness}
                async with session.post(f"{BASE_URL}/brightness", json=payload, timeout=5) as resp:
                    if resp.status == 200:
                        return f"成功将亮度设置为 {brightness}%"
                    else:
                        text = await resp.text()
                        return f"设置亮度失败: {text}"
        except Exception as e:
            return f"请求宿主机API发生异常: {str(e)}"

    @llm_tool(name="set_pc_volume", description="设置宿主机 Windows 电脑系统音量。")
    async def set_pc_volume(self, event: AstrMessageEvent, volume: int):
        """
        当用户想要设置、调整系统音量时调用。
        Args:
            volume (int): 目标音量值(0-100)。注意：这是必填参数，必须提供一个具体的整数！如果用户只是说“大点声”或“小点声”，请你自行推算一个合适的数值传入。
        """
        if not (0 <= volume <= 100):
            return "音量值必须在 0 到 100 之间。"
            
        try:
            async with aiohttp.ClientSession() as session:
                payload = {"value": volume}
                async with session.post(f"{BASE_URL}/volume", json=payload, timeout=5) as resp:
                    if resp.status == 200:
                        return f"成功将系统音量设置为 {volume}%"
                    else:
                        text = await resp.text()
                        return f"设置音量失败: {text}"
        except Exception as e:
            return f"请求宿主机API发生异常: {str(e)}"
    @llm_tool(name="lock_pc_screen", description="当用户想要锁定宿主机 Windows 电脑屏幕时调用此工具。")
    async def lock_pc_screen(self, event: AstrMessageEvent):
        try:
            async with aiohttp.ClientSession() as session:
                async with session.post(f"{BASE_URL}/lock", timeout=5) as resp:
                    if resp.status == 200:
                        return "电脑已成功锁屏。"
                    else:
                        text = await resp.text()
                        return f"锁屏失败: {text}"
        except Exception as e:
            return f"请求宿主机API发生异常: {str(e)}"

    @llm_tool(name="set_pc_mute", description="设置宿主机 Windows 电脑的静音状态。")
    async def set_pc_mute(self, event: AstrMessageEvent, is_mute: bool):
        """
        当用户想要将电脑静音或取消静音时调用此工具。
        Args:
            is_mute (bool): 传入 True 表示需要静音，传入 False 表示需要取消静音。
        """
        try:
            async with aiohttp.ClientSession() as session:
                payload = {"value": is_mute}
                async with session.post(f"{BASE_URL}/mute", json=payload, timeout=5) as resp:
                    if resp.status == 200:
                        state_str = "静音" if is_mute else "取消静音"
                        return f"成功将电脑设置为 {state_str} 状态"
                    else:
                        text = await resp.text()
                        return f"设置静音状态失败: {text}"
        except Exception as e:
            return f"请求宿主机API发生异常: {str(e)}"
