//go:build hongguolive

package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
)

// TestHongguoLiveVideoModel 用完整签名真实请求一次播放接口。
// 仅在 -tags hongguolive 时编译，避免进入常规测试。
func TestHongguoLiveVideoModel(t *testing.T) {
	const videoID = "7517919645588802622"
	deviceID := newHongguoDeviceID()
	installID := newHongguoDeviceID()
	query := hongguoAppDeviceQuery(deviceID, installID)

	payload := map[string]any{
		"video_id": videoID,
		"biz_param": map[string]any{
			"detail_page_version":       0,
			"device_level":              3,
			"disable_digg_stat":         false,
			"need_all_video_definition": true,
			"need_mp4_align":            false,
			"use_os_player":             false,
			"use_server_dns":            false,
			"video_platform":            1024,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	request, err := http.NewRequest(http.MethodPost, hongguoAppBaseURL+"/novel/player/video_model/v1/", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("User-Agent", hongguoAppUserAgent)
	request.Header.Set("Accept", "application/json; charset=utf-8,application/x-protobuf")
	request.Header.Set("Content-Type", "application/json; charset=UTF-8")
	request.URL.RawQuery = hongguoStableQuery(query, body, uint32(now.Unix()), now.UnixMilli())
	signHongguoRequest(request, body, now)

	t.Logf("branch=%d", hongguoBranchOf(request.URL.RawQuery, body, uint32(now.Unix())))
	t.Logf("deviceProto=%d bodyLen=%d queryLen=%d", len(hongguoDeviceProto("1234567890123456")), len(body), len(request.URL.RawQuery))
	t.Logf("x-gorgon=%s", request.Header.Get("x-gorgon"))
	t.Logf("x-medusa len=%d", len(request.Header.Get("x-medusa")))
	t.Logf("x-helios len=%d", len(request.Header.Get("x-helios")))

	client := &http.Client{Timeout: 25 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer response.Body.Close()
	content, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	t.Logf("HTTP %d, body=%d bytes", response.StatusCode, len(content))
	if len(content) == 0 {
		t.Fatalf("响应为空 —— 签名未被接受")
	}
	if len(content) < 300 {
		t.Logf("body: %s", string(content))
	}

	var result struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(content, &result); err != nil {
		t.Fatalf("响应不是 JSON: %s", string(content[:min(len(content), 200)]))
	}
	t.Logf("code=%d message=%s", result.Code, result.Message)
	if result.Code != 0 {
		t.Fatalf("接口拒绝: code=%d %s", result.Code, result.Message)
	}

	var data struct {
		VideoHeight int    `json:"video_height"`
		VideoWidth  int    `json:"video_width"`
		VideoModel  string `json:"video_model"`
	}
	if err := json.Unmarshal(result.Data, &data); err != nil {
		t.Fatalf("data 解析失败: %v", err)
	}
	var model map[string]any
	if err := json.Unmarshal([]byte(data.VideoModel), &model); err != nil {
		t.Fatalf("video_model 解析失败: %v", err)
	}
	list, _ := model["video_list"].([]any)
	t.Logf("推荐 %dx%d，video_list 共 %d 档", data.VideoWidth, data.VideoHeight, len(list))
	for _, item := range list {
		entry, _ := item.(map[string]any)
		meta, _ := entry["video_meta"].(map[string]any)
		codec := mapString(entry, "codec_type")
		if codec == "" {
			codec = mapString(meta, "codec_type")
		}
		width := mapString(entry, "vwidth")
		height := mapString(entry, "vheight")
		if width == "" {
			width = mapString(meta, "vwidth")
		}
		if height == "" {
			height = mapString(meta, "vheight")
		}
		bitrate := mapString(entry, "bitrate")
		hasURL := mapString(entry, "main_url") != ""
		fmt.Printf("  档位 %sx%s codec=%s bitrate=%s url=%v\n", width, height, codec, bitrate, hasURL)
	}
	if len(list) == 0 {
		t.Fatalf("video_list 为空")
	}
}
