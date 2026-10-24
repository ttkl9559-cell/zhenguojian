//go:build hongguosearchprobe

package core

import (
	"context"
	"testing"
)

// TestHongguoSearchLive 真机网络验证红果搜索接口。
// 只在 -tags hongguosearchprobe 时编译，不进入常规测试。
func TestHongguoSearchLive(t *testing.T) {
	engine, err := newNativeEngine(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(engine.downloads.close)
	engine.downloader.cfg.Retries = 2

	for _, keyword := range []string{"榜二", "车粉人狠"} {
		entry, err := engine.downloader.searchHongguoDramas(context.Background(), keyword)
		if err != nil {
			t.Fatalf("%s 搜索失败: %v", keyword, err)
		}
		t.Logf("关键词 %q：共 %d 条，返回 %d 条，Limited=%v，Warning=%q", keyword, entry.Total, len(entry.Dramas), entry.Limited, entry.Warning)
		if entry.Warning != "" {
			t.Errorf("%s 出现告警：%s", keyword, entry.Warning)
		}
		if len(entry.Dramas) < 5 {
			t.Errorf("%s 结果过少：%d", keyword, len(entry.Dramas))
		}
		for index, drama := range entry.Dramas {
			if index >= 12 {
				break
			}
			t.Logf("  %2d. %s | %s | 封面=%v | %s", index+1, drama.DisplayTitle(), drama.SourceID, drama.Cover != "", drama.Remark)
		}
	}
}
