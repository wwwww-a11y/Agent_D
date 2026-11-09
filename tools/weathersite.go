package tools

import (
	"fmt"
	"net/url"
	"os/exec"
	"strings"
)

// cnWeatherCityCode 高德 adcode -> 中国天气网城市编码
// 说明：两套编码体系不同，adcode 是行政区划码(如新乡 410700)，
// 中国天气网用的是气象城市码(如新乡 101180301)，所以需要一张映射表。
// 表里没有的城市会自动兜底到百度天气搜索页，不会报错。
// 补充城市的方法：打开 https://www.weather.com.cn 找到对应城市页面，
// 页面地址里那串数字就是城市编码，按 "adcode": "编码" 加一行即可。
var cnWeatherCityCode = map[string]string{
	"110000": "101010100", // 北京
	"410100": "101180101", // 郑州
	"410200": "101180801", // 开封
	"410300": "101180901", // 洛阳
	"410400": "101180501", // 平顶山
	"410500": "101180201", // 安阳
	"410600": "101181201", // 鹤壁
	"410700": "101180301", // 新乡
	"410800": "101181101", // 焦作
	"410900": "101181301", // 濮阳
}

// cnWeatherCityName 城市名 -> 中国天气网城市编码
// 模型传给工具的参数有时是城市名而不是 adcode，这里再兜一层，避免落到搜索页
var cnWeatherCityName = map[string]string{
	"北京": "101010100",
	"郑州": "101180101",
	"开封": "101180801",
	"洛阳": "101180901",
	"平顶山": "101180501",
	"安阳": "101180201",
	"鹤壁": "101181201",
	"新乡": "101180301",
	"焦作": "101181101",
	"濮阳": "101181301",
}

// OpenWeatherSite 查询成功后，用系统默认浏览器打开第三方官方天气页面
func OpenWeatherSite(city, adcode string) error {
	page := WeatherPageURL(city, adcode)
	fmt.Printf("🌐 正在打开天气页面: %s\n", page)
	if err := openURL(page); err != nil {
		return fmt.Errorf("打开天气页面失败: %w", err)
	}
	return nil
}

// WeatherPageURL 拼第三方天气页地址：优先中国天气网，兜底百度天气搜索
func WeatherPageURL(city, adcode string) string {
	if code, ok := cnWeatherCityName[normalizeCityName(city)]; ok {
		return "https://www.weather.com.cn/weather/" + code + ".shtml"
	}
	if code, ok := cnWeatherCityCode[strings.TrimSpace(adcode)]; ok {
		return "https://www.weather.com.cn/weather/" + code + ".shtml"
	}
	return "https://www.baidu.com/s?wd=" + url.QueryEscape(city+"天气")
}

// normalizeCityName 去掉"市/县/区"等后缀，便于匹配映射表
func normalizeCityName(s string) string {
	s = strings.TrimSpace(s)
	for _, suffix := range []string{"特别行政区", "自治州", "地区", "市", "县", "区"} {
		s = strings.TrimSuffix(s, suffix)
	}
	return s
}

// openURL 调系统默认浏览器打开网址(Start 不等待，不阻塞 Agent 主流程)
func openURL(u string) error {
	// 首选 rundll32，绕过 cmd 对 & ? 等 URL 特殊字符的解析问题
	if err := exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start(); err == nil {
		return nil
	}
	// 兜底方案：cmd start
	if err := exec.Command("cmd", "/c", "start", "", u).Start(); err == nil {
		return nil
	}
	return fmt.Errorf("无可用方式打开浏览器: %s", u)
}
