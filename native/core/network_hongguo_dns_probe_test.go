//go:build hongguodns

package core

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

// TestHongguoDohFallback 模拟「系统 DNS 返回了一个握手即复位的坏节点」，
// 验证 cdnTransport 能通过 DoH 换到可用节点。
func TestHongguoDohFallback(t *testing.T) {
	dead := "123.53.183.137:443"
	// 无论请求哪个域名，都硬连到已知坏节点上。
	transport := &http.Transport{
		TLSClientConfig:   &tls.Config{},
		ForceAttemptHTTP2: false,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 4 * time.Second}).DialContext(ctx, "tcp", dead)
		},
	}
	cdn := newCDNTransport(transport, newDNSResolver(&http.Transport{}))
	client := &http.Client{Transport: cdn, Timeout: 40 * time.Second}

	request, err := http.NewRequest(http.MethodGet, hongguoBaseURL+"/incent_resource/suggestion?app_id=8662&query=%E6%88%91&count=3", nil)
	if err != nil {
		t.Fatalf("构造请求失败: %v", err)
	}
	request.Header.Set("User-Agent", userAgent)
	request.Header.Set("Referer", hongguoBaseURL+"/")
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("兜底失败: %v", err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	t.Logf("HTTP %d, %d 字节, 前 120 字节: %.120s", response.StatusCode, len(body), string(body))
	if response.StatusCode != http.StatusOK {
		t.Fatalf("状态码异常: %d", response.StatusCode)
	}
	t.Logf("命中节点: %s", response.Request.URL.Host)

	for _, path := range []string{"/search/%E6%88%91%E6%9C%AC%E5%9B%9B%E5%8D%81%E5%B2%81", "/"} {
		pageRequest, err := http.NewRequest(http.MethodGet, hongguoBaseURL+path, nil)
		if err != nil {
			t.Fatalf("构造请求失败: %v", err)
		}
		pageRequest.Header.Set("User-Agent", userAgent)
		pageRequest.Header.Set("Referer", hongguoBaseURL+"/")
		pageResponse, err := client.Do(pageRequest)
		if err != nil {
			t.Fatalf("%s 兜底失败: %v", path, err)
		}
		pageBody, _ := io.ReadAll(io.LimitReader(pageResponse.Body, 512))
		pageResponse.Body.Close()
		t.Logf("%s -> HTTP %d, 前 80 字节: %.80s", path, pageResponse.StatusCode, string(pageBody))
		if pageResponse.StatusCode != http.StatusOK {
			t.Fatalf("%s 状态码异常: %d", path, pageResponse.StatusCode)
		}
	}
}
