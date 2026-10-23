package core

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
)

var nonPublicCDNNetworks = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"), netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"), netip.MustParsePrefix("2001::/32"),
	netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("2002::/16"),
}

// 红果网页站（hongguoduanju.com）也放进兜底名单：
// 它的 A 记录里混着大量握手即复位的坏节点（1.194.209.x / 123.53.183.x 等），
// 而系统 DNS 只返回其中一个。一旦被分配到坏节点，搜索、站源更新全部报
// "read: connection reset by peer"，但播放走的是 App 接口不受影响。
// 带 edns_client_subnet=8.8.8.0/24 查到的节点（111.47.131.x 等）实测可用，
// 交给 cdnTransport 逐个试连就能自愈。
func protectedCDNHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, domain := range []string{"tuafjz.cn", "zdmhyg.cn", "lkkwip.cn", "bnfuiu.cn", "huangguoai.fit", "hongguoduanju.com"} {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

func validateCDNAddresses(addresses []string) error {
	if len(addresses) == 0 {
		return errors.New("CDN 没有可用的公网地址")
	}
	for _, value := range addresses {
		address, err := netip.ParseAddr(value)
		if err != nil || address.Zone() != "" {
			return errors.New("CDN 返回无效地址")
		}
		address = address.Unmap()
		if !address.IsGlobalUnicast() || address.IsPrivate() {
			return fmt.Errorf("CDN 返回非公网地址：%s", address)
		}
		for _, network := range nonPublicCDNNetworks {
			if network.Contains(address) {
				return fmt.Errorf("CDN 返回保留地址：%s", address)
			}
		}
	}
	return nil
}
