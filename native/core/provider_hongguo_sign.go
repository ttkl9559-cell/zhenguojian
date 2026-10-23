package core

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"math/bits"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"
)

// 红果/番茄 App 接口的完整设备签名。
//
// 旧实现只发 X-Gorgon 的简化变体，并把 X-Argus 写死成 "1"，服务端对这种请求
// 已改为静默返回 HTTP 200 + 空 body，表现为「App 接口未返回有效数据」。
// 这里补齐官方客户端实际携带的整套头部：
// X-Gorgon / X-Argus / X-Helios / X-Ladon / X-Medusa / X-Khronos / X-SS-STUB。
//
// 算法与 jackotom/hongguo-drama（GPL-3.0）的 src/native/app-source.js 对齐，
// 其来源为 woshishiq1/drpys。下列常量是协议参数，不是用户凭据。

func newHongguoDeviceID() string {
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	// 官方客户端的合成 ID 是 16 位（10^15 起），位数也算指纹的一部分。
	return strconv.FormatUint(1_000_000_000_000_000+binary.BigEndian.Uint64(random[:])%9_000_000_000_000_000, 10)
}

// 进程级常量，与 JS 的 PROCESS_NONCE / PROCESS_STARTED 对应。
var (
	hongguoProcessNonce   = hongguoBase64URL(18)
	hongguoProcessStarted = time.Now().UnixMilli()
)

func hongguoBase64URL(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "nonce"
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

func hongguoRandUint32() uint32 {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return uint32(time.Now().UnixNano())
	}
	return binary.BigEndian.Uint32(b[:])
}

func hongguoLe32(value uint32) []byte {
	out := make([]byte, 4)
	binary.LittleEndian.PutUint32(out, value)
	return out
}

func hongguoBe32(value uint32) []byte {
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, value)
	return out
}

func hongguoMd5Sum(data []byte) []byte {
	sum := md5.Sum(data)
	return sum[:]
}

// hongguoEscapeQuery 按 encodeURIComponent 的规则转义。
// 注意 '*' 不转义：net/url 会把它写成 %2A，导致 query 与官方客户端不一致。
func hongguoEscapeQuery(value string) string {
	var out []byte
	for i := 0; i < len(value); i++ {
		c := value[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9',
			c == '-', c == '_', c == '.', c == '~', c == '*':
			out = append(out, c)
		default:
			out = append(out, '%', hexUpperTable[c>>4], hexUpperTable[c&15])
		}
	}
	return string(out)
}

const hexUpperTable = "0123456789ABCDEF"

func hongguoEncodeQuery(query url.Values) string {
	keys := make([]string, 0, len(query))
	for key := range query {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var out []byte
	for _, key := range keys {
		for _, value := range query[key] {
			if len(out) > 0 {
				out = append(out, '&')
			}
			out = append(out, hongguoEscapeQuery(key)...)
			out = append(out, '=')
			out = append(out, hongguoEscapeQuery(value)...)
		}
	}
	return string(out)
}

// hongguoStableQuery 生成不会被服务端丢弃的 query。
//
// 签名分支 branch==1 的变体服务端不认，JS 侧靠抖动 _rticket 避开；最多试 32 次。
func hongguoStableQuery(query url.Values, body []byte, khronos uint32, ticketBase int64) string {
	value := ""
	for offset := int64(0); offset < 32; offset++ {
		requestTicket := ticketBase + offset
		query.Set("ts", strconv.FormatUint(uint64(khronos), 10))
		query.Set("_rticket", strconv.FormatInt(requestTicket, 10))
		value = hongguoEncodeQuery(query)
		if hongguoBranchOf(value, body, khronos) != 1 {
			break
		}
	}
	return value
}

// signHongguoRequest 为一次 App 接口请求写入完整签名头。
// request.URL.RawQuery 必须已经由 hongguoStableQuery 生成。
func signHongguoRequest(request *http.Request, body []byte, now time.Time) {
	khronos := uint32(now.Unix())
	rawQuery := request.URL.RawQuery
	params := request.URL.Query()
	deviceID := params.Get("device_id")
	versionName := params.Get("version_name")
	requestTicket := params.Get("_rticket")
	if requestTicket == "" {
		requestTicket = strconv.FormatInt(now.UnixMilli(), 10)
	}

	request.Header.Set("x-ss-stub", hongguoHexUpper(hongguoMd5Sum(body)))
	request.Header.Set("x-gorgon", hongguoXGorgon(rawQuery, body, khronos, hongguoRandUint32()&0xffff))
	request.Header.Set("x-khronos", strconv.FormatUint(uint64(khronos), 10))
	request.Header.Set("x-ladon", base64.StdEncoding.EncodeToString(hongguoBe32(khronos)))
	request.Header.Set("x-argus", base64.StdEncoding.EncodeToString(hongguoLe32(khronos)))
	request.Header.Set("x-helios", hongguoHelios(khronos))
	request.Header.Set("x-medusa", hongguoBuildMedusa(rawQuery, body, khronos, deviceID, versionName))
	request.Header.Set("x-ss-req-ticket", requestTicket)
	request.Header.Set("x-tt-request-tag", "t=0;n=0")
	request.Header.Set("sdk-version", "2")
	request.Header.Set("passport-sdk-version", "50561")
	request.Header.Set("x-vc-bdturing-sdk-version", "3.7.2.cn")
	request.Header.Set("x-xs-from-web", "0")
}

func hongguoHexUpper(data []byte) string {
	encoded := make([]byte, hex.EncodedLen(len(data)))
	hex.Encode(encoded, data)
	for i := range encoded {
		if encoded[i] >= 'a' && encoded[i] <= 'f' {
			encoded[i] -= 32
		}
	}
	return string(encoded)
}

func hongguoHexBytes(value string) []byte {
	decoded, err := hex.DecodeString(value)
	if err != nil {
		return nil
	}
	return decoded
}

// ---------- 分支选择 ----------

func hongguoGetIv(iv uint32, data []byte) uint32 {
	value := iv
	for i := 0; i < len(data); i++ {
		if i&1 == 0 {
			value = (value >> 4) ^ value ^ (value << 6) ^ uint32(data[i])
		} else {
			value = ^((value >> 7) ^ value ^ (uint32(data[i]) | (value << 12)))
		}
	}
	return value
}

func hongguoSumMd5(data []byte) uint32 {
	check := uint32(0x20220420)
	for i := 0; i < 12; i++ {
		var temp uint32
		if i&1 == 0 {
			temp = (check >> 3) ^ check
		} else {
			temp = (check >> 5) ^ check
		}
		if i&1 == 0 {
			check = uint32(data[i]) ^ (check << 7)
		} else {
			check = uint32(data[i]) | (check << 11)
		}
		if i&1 != 0 {
			check = ^check
		}
		check = check ^ temp
	}
	return (check | 4) ^ 0x1000000
}

func hongguoBranchOf(rawQuery string, body []byte, khronos uint32) int {
	bodyDigest := hongguoMd5Sum(body)
	queryDigest := hongguoSM3Sum([]byte(rawQuery))
	iv := hongguoGetIv(hongguoGetIv(hongguoGetIv(0x20230928, queryDigest), bodyDigest), hongguoLe32(khronos))
	low := int(iv & 15)
	return low - (((low * 171) >> 9) * 3)
}

// ---------- F13（Medusa 内部的摘要层） ----------

var hongguoBranch2SV = [64]uint32{
	0xa7aefe20, 0x7149f1d6, 0x47e4ca07, 0xe9b58f67, 0x93b924de, 0xc614d0f5, 0x38afe0ef, 0xb2bbad73,
	0xe24444c3, 0x9d3aec9b, 0xdf7b37e4, 0xd8b16d40, 0xf8ac31b8, 0x76b9a90b, 0x31d833ee, 0x953fce64,
	0x353595a4, 0x4609c13b, 0x36925008, 0x8c6d0925, 0x5df5c177, 0x1cfbf52b, 0x8a4fa7f0, 0x114ca35e,
	0x8193f984, 0x7a7a8733, 0x316ab4d5, 0x3c20cfc9, 0xa6d84453, 0x3a18500c, 0x798ec47a, 0x97a76b28,
	0x66c4ff96, 0x51716443, 0x0dd2fc3b, 0xb5696da7, 0xbbeb3ac5, 0x5c53d204, 0xd32608ce, 0x7279b9ec,
	0xf4188ecf, 0xf7d793db, 0x332cc491, 0xab76ae15, 0x9bebe727, 0x18a01384, 0x5be9f8a7, 0x5f90a754,
	0x39b663c0, 0x36673c83, 0x7c92f514, 0x9d7d94d7, 0xe2e8d9aa, 0x5f7e9ea9, 0x7abd4551, 0x569e05da,
	0x40a25632, 0x3df5a9a5, 0xbab37d80, 0x454286dc, 0x3f5d4e78, 0x3d7b75d, 0xb1fe4af7, 0xa5ab26a3,
}

const hongguoBranch2OrdersHex = "0f0704000908030a060b050d0e010c020f05080c0009020103070e060b0a0d04060500070c000a04080f010b0d09020e060b02050403080101070a000d0c090e0d070e0f0b0208030c0509010004060a0d0902060f0b0a040807000c0503010e0c090f07060f030e020d0405010b0a000c050a090e0802040407030f01060b000b0f0408020a0700090d06010e03050c0b060a0508020c03070f0e090d0001040906080f050800040a0b030d01020c0e090d0c0604070a03030f00080105020e01000d0f090a0b0e0402080703060c0501080a0c0f0905060b000304020e070d04080f000c0f0e0d0a01060207090503040205080d0b0a0606000e0f070c09030a08040f000b01060d0c0709030e05020a070b050f00020e0108030d0c0609040d070f08050f06040b0a0e0c090002030d0c020704010b0e0e08060f09050003"

var hongguoBranch2Orders = hongguoHexBytes(hongguoBranch2OrdersHex)

func hongguoMd5V3Step(kind int, a, b, c, d, m, shift, constant uint32) uint32 {
	var f uint32
	switch kind {
	case 0:
		f = (b & c) | (^b & d)
	case 1:
		f = (b & d) | (c & ^d)
	case 2:
		f = b ^ c ^ d
	default:
		f = c ^ (b | ^d)
	}
	return b + bits.RotateLeft32(a+f+m+constant, int(shift))
}

func hongguoMd5SumV3(message []byte, countV2 uint32, orders []byte, countV1 uint32) []byte {
	sv := make([]uint32, len(hongguoBranch2SV))
	for i, value := range hongguoBranch2SV {
		sv[i] = bits.RotateLeft32(value, -int(countV1&31))
	}
	start := [4]uint32{
		bits.RotateLeft32(0x79e0f2fb, -int(countV2&31)),
		bits.RotateLeft32(0xc8b52570, -int(countV2&31)),
		bits.RotateLeft32(0xebc2f8cd, -int(countV2&31)),
		bits.RotateLeft32(0x7c104d93, -int(countV2&31)),
	}
	endCount := (countV2 + 6) & 255
	end := [4]uint32{
		bits.RotateLeft32(0x19be4866, -int(endCount&31)),
		bits.RotateLeft32(0xe85986b4, -int(endCount&31)),
		bits.RotateLeft32(0xe19b326e, -int(endCount&31)),
		bits.RotateLeft32(0x71d1d7d4, -int(endCount&31)),
	}
	m := make([]uint32, 16)
	for i := 0; i < 16; i++ {
		m[i] = binary.LittleEndian.Uint32(message[i*4 : i*4+4])
	}
	a, b, c, d := start[0], start[1], start[2], start[3]
	roundShifts := [4][16]uint32{
		{7, 12, 17, 22, 7, 12, 17, 22, 7, 12, 17, 22, 7, 12, 17, 22},
		{5, 9, 14, 20, 5, 9, 14, 20, 5, 9, 14, 20, 5, 9, 14, 20},
		{4, 11, 16, 23, 4, 11, 16, 23, 4, 11, 16, 23, 4, 11, 16, 23},
		{6, 10, 15, 21, 6, 10, 15, 21, 6, 10, 15, 21, 6, 10, 15, 21},
	}
	for kind := 0; kind < 4; kind++ {
		offset := kind * 16
		for i := 0; i < 16; i++ {
			orderIndex := offset + i
			if orderIndex >= len(orders) {
				return nil
			}
			word := m[orders[orderIndex]]
			shift := roundShifts[kind][i]
			constant := sv[orderIndex]
			switch i & 3 {
			case 0:
				a = hongguoMd5V3Step(kind, a, b, c, d, word, shift, constant)
			case 1:
				d = hongguoMd5V3Step(kind, d, a, b, c, word, shift, constant)
			case 2:
				c = hongguoMd5V3Step(kind, c, d, a, b, word, shift, constant)
			default:
				b = hongguoMd5V3Step(kind, b, c, d, a, word, shift, constant)
			}
		}
	}
	ret := make([]byte, 16)
	binary.LittleEndian.PutUint32(ret[0:], (start[0]+a)^end[0])
	binary.LittleEndian.PutUint32(ret[4:], (start[1]+b)^end[1])
	binary.LittleEndian.PutUint32(ret[8:], (start[2]+c)^end[2])
	binary.LittleEndian.PutUint32(ret[12:], (start[3]+d)^end[3])
	return append(ret, hongguoLe32(hongguoSumMd5(ret[:12]))...)
}

func hongguoBranch2F13(iv uint32, queryDigest, bodyDigest, tsBytes []byte, khronos uint32) []byte {
	v := (iv & 13) * 86
	ivV0 := ((v >> 15) & 255) + ((v >> 8) & 255)
	seeds := [5]uint32{0x8980f29b, 0xeb549c7f, 0xb08726db, 0xd40cb5e6, 0xe8f559e4}
	if int(ivV0) >= len(seeds) {
		return nil
	}
	n1 := seeds[ivV0]
	countV1 := (n1 + khronos + 1) & 255
	countV2 := n1 + khronos
	shift := (countV2 + 5) & 7
	seed := [8]byte{0x84, 0x96, 0x77, 0x9d, 0xd4, 0x15, 0x0b, 0xf8}
	pad := make([]byte, 8)
	for i, value := range seed {
		pad[i] = byte(((uint16(value) | (uint16(value) << 8)) >> shift) & 255)
	}
	input := make([]byte, 0, 64)
	input = append(input, queryDigest...)
	input = append(input, bodyDigest...)
	input = append(input, tsBytes...)
	input = append(input, pad...)
	input = append(input, 0xa0, 0x01, 0x00, 0x00)
	start := int(ivV0) * 64
	if len(hongguoBranch2Orders) < start+64 {
		return nil
	}
	return hongguoMd5SumV3(input, countV2, hongguoBranch2Orders[start:start+64], countV1)
}

func hongguoHashF13(queryDigest, bodyDigest, tsBytes []byte, khronos uint32) []byte {
	iv := hongguoGetIv(hongguoGetIv(hongguoGetIv(0x20230928, queryDigest), bodyDigest), tsBytes)
	low := int(iv & 15)
	ivV0 := (low * 171) >> 9
	branch := low - ivV0*3
	if branch == 2 {
		return hongguoBranch2F13(iv, queryDigest, bodyDigest, tsBytes, khronos)
	}
	if branch != 0 {
		combined := make([]byte, 0, len(queryDigest)+len(bodyDigest))
		combined = append(combined, queryDigest...)
		combined = append(combined, bodyDigest...)
		out := make([]byte, 0, 24)
		out = append(out, queryDigest[:16]...)
		out = append(out, bodyDigest...)
		out = append(out, hongguoLe32(hongguoSumMd5(combined))...)
		return out
	}

	ivV1List := [6]uint32{0xc4a78580, 0xb3c0fd39, 0xc58c5686, 0xc9aa3ba7, 0xf5a7adf2, 0x963c2ed1}
	if ivV0 >= len(ivV1List) {
		return nil
	}
	ivV1 := ivV1List[ivV0]
	countV1 := (ivV1 + khronos + 1) & 255
	countV2 := ivV1 + khronos
	tt02 := [64]uint32{
		0xebb64faf, 0x07aadcc2, 0xcf3187bf, 0xe01138ff, 0x6d0bfcff, 0x5a30a3be, 0xb41ad638, 0x34180eb8,
		0xf233eb6f, 0xb1a584cc, 0xccc30dc7, 0x47d1db51, 0xd55653de, 0x70a84fa1, 0x57473c12, 0xf76f0288,
		0x2c077f0a, 0xda0dcad0, 0xfbb86f6c, 0xfdc4cf00, 0x688a020d, 0xe676c6a6, 0x8cd6338b, 0x1a3c8d0e,
		0xcce8b06b, 0x6ad0ed0b, 0xa0522717, 0xdc71ac83, 0x2285db71, 0xd5b4dda6, 0x736f8650, 0x6560306c,
		0x617ce2a6, 0xe423417e, 0x0a40e143, 0x544e4032, 0x88dffb2a, 0x716c1ae0, 0x4c467a88, 0x05b23bb3,
		0xe1d0b866, 0xbaa3dcb8, 0xae3374d3, 0xc3381a50, 0x1702f75b, 0xfe6da368, 0xf0b4cf48, 0x4e0ffbb8,
		0x72aad10d, 0x26c53a3d, 0xf2bce0f6, 0xb4557581, 0x4a257fdd, 0x8c3182a2, 0xab0b3b86, 0x3d5dfb14,
		0x4f103634, 0xd37b52d7, 0x444eff16, 0xeb0a33d1, 0x6ca86f6e, 0x00284ba7, 0x08387cfa, 0x5fb37586,
	}
	tt := make([]uint32, 64)
	for i, value := range tt02 {
		tt[i] = bits.RotateLeft32(value, -int(countV1&31))
	}
	n0 := (countV2 + 2) & 7
	seed := [4]byte{0xfa, 0x45, 0x61, 0xd7}
	pad := make([]byte, 4)
	for i, value := range seed {
		pad[i] = byte(((uint16(value) | (uint16(value) << 8)) >> n0) & 255)
	}
	data := make([]byte, 0, 64)
	data = append(data, queryDigest...)
	data = append(data, bodyDigest...)
	data = append(data, tsBytes...)
	data = append(data, pad...)
	data = append(data, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0xa0)
	di := make([]uint32, 0, 128)
	for i := 0; i < len(data); i += 4 {
		di = append(di, binary.BigEndian.Uint32(data[i:i+4]))
	}
	di0 := di[0]
	for i := 0; i < 112; i++ {
		di1 := di[i+1]
		di14 := di[i+14]
		r1 := bits.RotateLeft32(di1, 14) ^ bits.RotateLeft32(di1, 25) ^ (di1 >> 3)
		r2 := bits.RotateLeft32(di14, 13) ^ bits.RotateLeft32(di14, 15) ^ (di14 >> 10)
		di = append(di, di0+di[i+9]+r1+r2)
		di0 = di1
	}
	initValues := [8]uint32{0x7aba4fc8, 0x67166507, 0x6403fa00, 0x340f512f, 984304912, 3005047866, 2874125293, 2152413264}
	chosenTable := [6][11]int{
		{101, 5, 7, 6, 3, 2, 1, 0, 5, 4, 3},
		{96, 0, 6, 7, 5, 3, 2, 1, 5, 4, 4},
		{96, 7, 6, 2, 1, 4, 0, 5, 4, 3, 5},
		{99, 3, 6, 2, 4, 5, 1, 0, 0, 7, 6},
		{96, 0, 5, 6, 7, 3, 1, 2, 5, 4, 4},
		{100, 2, 0, 3, 5, 4, 6, 7, 2, 1, 5},
	}
	initRotated := make([]uint32, 8)
	for i, value := range initValues {
		initRotated[i] = bits.RotateLeft32(value, -int(countV2&31))
	}
	chosen := chosenTable[ivV0]
	d := append([]uint32{}, initRotated...)
	for i := 0; i < chosen[0]; i++ {
		base := ivV1 + uint32(i)
		n0v := di[base&127]
		n1 := ((d[chosen[3]] ^ d[chosen[4]]) & d[chosen[1]]) ^ d[chosen[3]]
		n2 := bits.RotateLeft32(d[chosen[1]], 26) ^ bits.RotateLeft32(d[chosen[1]], 21) ^ bits.RotateLeft32(d[chosen[1]], 7)
		n4 := n0v + n1 + n2 + tt[base&63] + d[chosen[5]]
		n5 := bits.RotateLeft32(d[chosen[2]], 30) ^ bits.RotateLeft32(d[chosen[2]], 19) ^ bits.RotateLeft32(d[chosen[2]], 10)
		n6 := (d[chosen[2]] & d[chosen[6]]) | ((d[chosen[2]] | d[chosen[6]]) & d[chosen[7]])
		n7 := n5 + n6
		old := d[chosen[9]]
		last := d[len(d)-1]
		copy(d[1:], d[:len(d)-1])
		d[0] = last
		d[chosen[10]] = n7 + n4
		d[chosen[8]] = old + n4
	}
	ret := make([]byte, 32)
	for i := 0; i < 8; i++ {
		binary.BigEndian.PutUint32(ret[i*4:], d[i]+initRotated[i])
	}
	folded := make([]byte, 16)
	for i := 0; i < 16; i++ {
		folded[i] = ret[i] ^ ret[16+i]
	}
	return append(folded, hongguoLe32(hongguoSumMd5(folded))...)
}

// ---------- X-Gorgon ----------

func hongguoReverseBits(value byte) byte {
	var result byte
	for i := 0; i < 8; i++ {
		result = (result << 1) | ((value >> uint(i)) & 1)
	}
	return result
}

func hongguoRC4Gorgon(data, key []byte) []byte {
	s := make([]byte, 256)
	for i := range s {
		s[i] = byte(i)
	}
	j := 0
	for i := 0; i < 256; i++ {
		j = (j + int(s[i]) + int(key[i%len(key)])) & 255
		s[i], s[j] = s[j], s[i]
	}
	i := 0
	j = 0
	out := make([]byte, len(data))
	for index, value := range data {
		i = (i + 1) & 255
		x := s[i]
		j = (j + int(x)) & 255
		y := s[j&255]
		s[i] = y
		out[index] = value ^ s[(int(y)+int(y))&255]
	}
	return out
}

func hongguoXGorgon(rawQuery string, body []byte, timestamp, random uint32) string {
	sdk := make([]byte, 4)
	binary.LittleEndian.PutUint32(sdk, 67503104)
	input := make([]byte, 0, 20)
	input = append(input, hongguoMd5Sum([]byte(rawQuery))[:4]...)
	if body != nil {
		input = append(input, hongguoMd5Sum(body)[:4]...)
	} else {
		input = append(input, 0, 0, 0, 0)
	}
	input = append(input, 0, 0, 0, 0)
	input = append(input, sdk...)
	input = append(input, hongguoBe32(timestamp)...)

	key := []byte{0x4a, 0x40, 0x16, byte((random >> 8) & 255), 0x47, 0x6c, 0x01, byte(random & 255)}
	out := hongguoRC4Gorgon(input, key)
	for i := range out {
		value := (out[i] >> 4) | (out[i] << 4)
		next := out[(i+1)%len(out)]
		out[i] = ^(hongguoReverseBits((next^value)&255) ^ 20)
	}
	signature := []byte{0x84, 0x04, byte(random & 255), byte((random >> 8) & 255), 0x40, 0x01}
	signature = append(signature, out...)
	return hex.EncodeToString(signature)
}

// ---------- protobuf ----------

type hongguoProtoField struct {
	tag   uint64
	value any
	kind  string
}

func hongguoVarint(value uint64) []byte {
	out := make([]byte, 0, 10)
	for value > 127 {
		out = append(out, byte(value&127)|128)
		value >>= 7
	}
	return append(out, byte(value))
}

func hongguoZigzag(value int64) uint64 {
	if value < 0 {
		return uint64(-value)*2 - 1
	}
	return uint64(value) * 2
}

func hongguoProtoDelimited(tag uint64, body []byte) []byte {
	out := hongguoVarint((tag << 3) | 2)
	out = append(out, hongguoVarint(uint64(len(body)))...)
	return append(out, body...)
}

func hongguoProtoVarintField(tag uint64, value uint64) []byte {
	return append(hongguoVarint(tag<<3), hongguoVarint(value)...)
}

func hongguoProtoFieldBytes(field hongguoProtoField) []byte {
	if field.value == nil {
		return nil
	}
	switch value := field.value.(type) {
	case []byte:
		return hongguoProtoDelimited(field.tag, value)
	case string:
		if value == "" {
			return nil
		}
		return hongguoProtoDelimited(field.tag, []byte(value))
	case uint64:
		return hongguoProtoVarintField(field.tag, value)
	case int64:
		return hongguoProtoVarintField(field.tag, hongguoZigzag(value))
	case int:
		return hongguoProtoVarintField(field.tag, hongguoZigzag(int64(value)))
	case uint32:
		return hongguoProtoVarintField(field.tag, uint64(value))
	default:
		return nil
	}
}

func hongguoProto(fields []hongguoProtoField) []byte {
	out := make([]byte, 0, 256)
	for _, field := range fields {
		if field.kind == "float" {
			// 所有 float 字段的取值都是 0，wire type 5。
			out = append(out, hongguoVarint((field.tag<<3)|5)...)
			out = append(out, 0, 0, 0, 0)
			continue
		}
		out = append(out, hongguoProtoFieldBytes(field)...)
	}
	return out
}

func hongguoDeviceProto(deviceID string) []byte {
	return hongguoProto([]hongguoProtoField{
		{1, int64(1), "sint"},
		{2, int64(2), "sint"},
		{3, "8662", "string"},
		{4, deviceID, "string"},
		{5, hongguoProcessNonce, "string"},
		{6, "!noperm!", "string"},
		{7, int64(-888888), "sint"},
		{8, int64(-888888), "sint"},
		{9, int64(3), "sint"},
		{10, int64(-888888), "sint"},
		{11, "!notset!", "string"},
		{12, "Asia/Shanghai,8", "string"},
		{13, "zh_CN", "string"},
		{14, int64(4), "sint"},
		{16, 0, "float"},
		{17, 0, "float"},
		{18, 0, "float"},
		{19, 0, "float"},
		{20, 0, "float"},
		{21, 0, "float"},
		{22, "16", "string"},
		{23, int64(41), "sint"},
		{24, int64(36), "sint"},
		{25, hongguoProcessStarted, "sint"},
		{26, hongguoProcessStarted, "sint"},
		{27, hongguoProcessStarted, "sint"},
		{28, hongguoProcessStarted, "sint"},
		{29, int64(-1), "sint"},
		{30, "25053RT47C", "string"},
		{31, "Redmi", "string"},
		{32, "25053RT47C", "string"},
		{33, "25053RT47C", "string"},
		{34, "Xiaomi", "string"},
		{35, "Redmi", "string"},
		{36, "Redmi", "string"},
		{38, int64(31), "sint"},
	})
}

// ---------- 自定义 AES（Medusa 载荷） ----------

const hongguoAesSboxBase64 = "+n0Ia5xZs0sEXznQOEqRmQBnpiCf9U2Ccybu3xhmgzOAAxn72f6uqqmwUsYL83klTni0NqxdGieeiNu9PGPsSRXBMB/cuFbUbM3KCUPINaPvHvSW0vwOcnuUhNHqRVpiAj/TEoE0K91+5ijypUYTATsh9mE3KSoN7Yyvv51cuyR2D3XkU4nhmI2xmmVwT1RMWKtub4sjxAcRDLrPoKSO2AU9FLLadMPX577Wf95IFj6FkKFVt3dCIsmGUC4X+WQxLJvxbRxEaOPpqJOXyzJX6+Vxaq3AzMfF/WAdoi1Hp+JRaV56zgpBtpWP97mH4DoGEIq1+FvV8LyS/3wvwugbQOwb2r26mJEMsiuDQTRn+wrYdrVGBVlhI3WQhyrjUBVMrLF5667llUcEaPCGPVGLD8qO5LlO8hKCvA7V9+8oJc9bXelqVQLhM76T5/WtnT45JKji+hdX0HoNCDDWuKON/QeaxB5uImSX0h2wv0VmP2zd2yeApxHcpsVS+MC2yFwAc2B7oBkTqsk1SEvTpM2fmfMQREBUfin0Bh+iq6EvPPavhWI2IX9e3yAas7Tm/3KEj2UmlFp36kN4x0rMLBRrxuh0U/zUHM4xcAMYjJY4MonxOl/X+alpt2M3WMI7w3HLnpIBigtNiJu7T21v4P6lSd5WFgntnMEt7oF92XzRLkJbTcGmXepE/UVOG6E/0YnhfS+q26utWcuxzpooyeD2cDlK1/8w9d28VzsRjbLuALbmGlp8+d7EzS6Au7lMpZ+ECMZvQmzwJ+eLOpxR+2chdUExp8ogQyq3v9l68rX4jCwjg0+PYKAEEzcU4wHFY2ZcdIHfWL1okD3SszT0GZMyKdZJrg1L2AeerB4tC0C4crp2EHGo5FYdSP7lwkeR2ocmnR+Ia8CYviUJlzOjhRZef9xuVOn3qcjow3fQgivsAmKKkg4+sA8F8/GWeDiGNhg8JM8KtFPMYWWkx5TVFX5t73kiNRJqjlIGVXtGZFCV4gzt0xcDopuZ6xz8r9Rzafpf9ywev8jh8592gHFIqpStZPuJxmDDMrNN0uBE3V+oscdoIzTJbRJ/t+sVvqnReJOgDJKk10fjisJwqyZBmnmn2BSFj8BvVtCMEbkuPOKdzw7eA11GPs04Qw8zWtkaZWwiO/wwpojqN6K0jY5RnNZA7vn4hPSul+nKCkVnVwQvg1zVxcSCtqORmB9KrJaBbssbCQivGJVJfVTt+hYxOtq4ZvWl8f4QAQZ0zGPffCgl9s6yT4vlvIdpu4YhBwA25wtQWZsc6GJYGWHyvSdeuh3mmUI9DSq13Fsp8C1MU3tqc04/df9LoTUXVXI5INOw/e8C7Hd+5CvbkMEFnnrUUmskEw=="

var hongguoAesSbox = func() []byte {
	decoded, err := base64.StdEncoding.DecodeString(hongguoAesSboxBase64)
	if err != nil || len(decoded) != 1024 {
		return make([]byte, 1024)
	}
	return decoded
}()

type hongguoAesV3 struct {
	box   []byte
	con2  [4]int
	order [16]int
	keys  [][4]byte
}

func hongguoXorBytes(a, b []byte) []byte {
	length := len(a)
	if len(b) < length {
		length = len(b)
	}
	out := make([]byte, length)
	for i := 0; i < length; i++ {
		out[i] = a[i] ^ b[i]
	}
	return out
}

func hongguoNewAesV3(key []byte, khronos uint32) *hongguoAesV3 {
	wordSize := int(khronos & 3)
	aes := &hongguoAesV3{
		box:  hongguoAesSbox[wordSize*256 : (wordSize+1)*256],
		con2: [4][4]int{{1, 0, 2, 3}, {2, 0, 3, 1}, {0, 1, 3, 2}, {1, 0, 2, 3}}[wordSize],
	}
	aes.order = [4][16]int{
		{0, 9, 14, 11, 4, 13, 2, 7, 8, 1, 6, 15, 12, 5, 10, 3},
		{0, 9, 14, 15, 4, 13, 2, 7, 8, 1, 6, 3, 12, 5, 10, 11},
		{0, 9, 14, 7, 4, 13, 2, 11, 8, 1, 6, 3, 12, 5, 10, 15},
		{0, 9, 14, 11, 4, 13, 2, 7, 8, 1, 6, 15, 12, 5, 10, 3},
	}[wordSize]
	initValue := [4]uint32{0xca025ddc, 0x823dc546, 0xc9420583, 0xc298225f}[wordSize]
	initial := make([]byte, 16)
	for i := 0; i < 4; i++ {
		binary.LittleEndian.PutUint32(initial[i*4:], initValue)
	}
	material := append(hongguoXorBytes(initial, key), make([]byte, 32)...)
	rounds := uint32(8)
	for i := 4; i < 12; i++ {
		at := 4 * (i - 1)
		a, b, c, d := material[at], material[at+1], material[at+2], material[at+3]
		if i&3 == 0 {
			t := (uint32(int32(initValue)>>(rounds&24)) ^ uint32(aes.box[b])) & 255
			b = aes.box[c]
			c = aes.box[d]
			d = aes.box[a]
			a = byte(t)
		}
		rounds += 2
		material[at+4] = a ^ material[at-12]
		material[at+5] = b ^ material[at-11]
		material[at+6] = c ^ material[at-10]
		material[at+7] = d ^ material[at-9]
	}
	aes.keys = make([][4]byte, 12)
	for i := 0; i < 12; i++ {
		copy(aes.keys[i][:], material[i*4:i*4+4])
	}
	return aes
}

func hongguoAesMatrix(bytes []byte) [4][4]byte {
	var rows [4][4]byte
	for i := 0; i < 4; i++ {
		copy(rows[i][:], bytes[i*4:i*4+4])
	}
	return rows
}

func hongguoAesFlat(rows [4][4]byte) []byte {
	out := make([]byte, 16)
	for i := 0; i < 4; i++ {
		copy(out[i*4:], rows[i][:])
	}
	return out
}

func hongguoAesMix(rows *[4][4]byte) {
	xt := func(x byte) byte { return (x << 1) ^ (byte(0x1b) * (x >> 7)) }
	for i := 0; i < 4; i++ {
		t := rows[0][i] ^ rows[1][i] ^ rows[2][i] ^ rows[3][i]
		u := rows[0][i]
		rows[0][i] ^= t ^ xt(rows[0][i]^rows[1][i])
		rows[1][i] ^= t ^ xt(rows[1][i]^rows[2][i])
		rows[2][i] ^= t ^ xt(rows[2][i]^rows[3][i])
		rows[3][i] ^= t ^ xt(rows[3][i]^u)
	}
}

func (aes *hongguoAesV3) add(rows *[4][4]byte, keys [][4]byte) {
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			rows[i][j] ^= keys[i][j]
		}
	}
}

func (aes *hongguoAesV3) addCon(rows *[4][4]byte, keys [][4]byte) {
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			rows[i][j] ^= keys[i][aes.con2[j]]
		}
	}
}

func (aes *hongguoAesV3) sub(rows *[4][4]byte) {
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			rows[i][j] = aes.box[rows[i][j]]
		}
	}
	old := *rows
	for i := 0; i < 4; i++ {
		rows[i] = old[aes.con2[i]]
	}
}

func (aes *hongguoAesV3) shift(rows *[4][4]byte) {
	flat := hongguoAesFlat(*rows)
	for i := 0; i < 16; i++ {
		rows[i/4][i%4] = flat[aes.order[i]]
	}
}

func (aes *hongguoAesV3) shiftCon(rows *[4][4]byte) {
	old := *rows
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			rows[i][j] = old[i][aes.con2[j]]
		}
	}
}

func (aes *hongguoAesV3) block(value []byte) []byte {
	rows := hongguoAesMatrix(value)
	aes.addCon(&rows, aes.keys[0:4])
	for i := 1; i < 3; i++ {
		aes.sub(&rows)
		aes.shift(&rows)
		if i == 1 {
			aes.shiftCon(&rows)
			hongguoAesMix(&rows)
		}
		aes.addCon(&rows, aes.keys[i*4:i*4+4])
	}
	aes.add(&rows, aes.keys[4:8])
	return hongguoAesFlat(rows)
}

func (aes *hongguoAesV3) encrypt(data, iv []byte) []byte {
	source := append([]byte{}, data...)
	plaintext := make([]byte, 32)
	for i := 0; i < 31; i++ {
		at := i * 8
		n0 := (source[at] >> 4) & 2
		n1 := n0 | (source[at+1] & 64)
		n2 := n1 | ((source[at+2] >> 2) & 1)
		n3 := n2 | ((source[at+3] << 3) & 128)
		n4 := n3 | ((source[at+4] >> 1) & 4)
		n5 := n4 | ((source[at+5] << 3) & 16)
		n6 := n5 | ((source[at+6] << 5) & 32)
		plaintext[i] = n6 | ((source[at+7] >> 4) & 8)
	}
	plaintext[31] = 1
	previous := append([]byte{}, iv...)
	keyStream := make([]byte, 0, 32)
	for i := 0; i < len(plaintext); i += 16 {
		value := aes.block(hongguoXorBytes(plaintext[i:i+16], previous))
		keyStream = append(keyStream, value...)
		previous = value
	}
	out := append([]byte{}, source...)
	for i := 0; i < 31; i++ {
		at := i * 8
		k := keyStream[i]
		out[at] = out[at]&0xdf | ((k << 4) & 32)
		out[at+1] = out[at+1]&0xbf | (k & 64)
		out[at+2] = out[at+2]&0xfb | ((k << 2) & 4)
		out[at+3] = out[at+3]&0xef | ((k >> 3) & 16)
		out[at+4] = out[at+4]&0xf7 | ((k + k) & 8)
		out[at+5] = out[at+5]&0xfd | ((k >> 3) & 2)
		out[at+6] = out[at+6]&0xfe | ((k >> 5) & 1)
		out[at+7] = out[at+7]&0x7f | ((k << 4) & 128)
	}
	return append([]byte{keyStream[len(keyStream)-1]}, out...)
}

// ---------- Medusa ----------

func hongguoXmxor(data, key []byte) []byte {
	encoded := make([]byte, len(data))
	for i := 0; i < len(data); i++ {
		at := (i * 4) & 28
		d0 := key[at]
		d1 := key[at+1]
		value := ((data[i] << 4) | (data[i] >> 4)) + d0
		value = ^value ^ d1
		value = (value << 3) | (value >> 5)
		value = value + d1
		value = value ^ d0
		encoded[len(data)-i-1] = ^value
	}
	length := len(encoded)
	last := encoded[length-1] ^ encoded[length-2]
	first := encoded[0]
	encoded[0] = ^last + first
	encoded[1] = encoded[0]^encoded[length-1]^254 + encoded[1]
	rotated := (encoded[1] << 3) | (encoded[1] >> 5)
	encoded[2] = encoded[2] + ((last - first) ^ rotated ^ 2)
	for i := 0; i < length-4; i++ {
		rotated := (encoded[i+2] << 3) | (encoded[i+2] >> 5)
		temp := rotated ^ encoded[i+1] ^ byte(i+3)
		encoded[i+3] = ^temp + encoded[i+3]
	}
	encoded[length-1] ^= encoded[length-2]
	var sum byte
	for i := 0; i < length-1; i++ {
		sum += encoded[i+1]
	}
	encoded[0] = (encoded[0] ^ encoded[1]) + sum
	return encoded
}

func hongguoKeyHash(signKey []byte, random uint32) ([]byte, []byte) {
	input := make([]byte, 0, len(signKey)*2+4)
	input = append(input, signKey...)
	input = append(input, hongguoLe32(random)...)
	input = append(input, signKey...)
	digest := hongguoSM3Sum(input)
	d1 := uint32((random >> 16) & 255)
	d2 := ((d1 << 11) | (random >> 24)) ^ (d1 >> 5) ^ d1
	return digest, hongguoLe32(^d2)
}

var hongguoMedusaSignKey = hongguoHexBytes("8ebdfa3806ecc5cee79423e6029ed82540bc2218bb7eaef71cb691f7aa8aa2f5")

func hongguoBuildMedusa(rawQuery string, body []byte, khronos uint32, deviceID, versionName string) string {
	if deviceID == "" {
		deviceID = newHongguoDeviceID()
	}
	if versionName == "" {
		versionName = "7.3.5.32"
	}
	bodyDigest := hongguoMd5Sum(body)
	queryDigest := hongguoSM3Sum([]byte(rawQuery))
	tsBytes := hongguoLe32(khronos)
	queryBodyTs := hongguoHashF13(queryDigest, bodyDigest, tsBytes, khronos)
	if len(queryBodyTs) < 20 {
		return ""
	}

	nested := hongguoProto([]hongguoProtoField{
		{1, int64(111), "sint"},
		{2, int64(10), "sint"},
		{3, int64(694367), "sint"},
		{5, int64(586952199), "sint"},
	})
	messageRand := hongguoRandUint32()
	envLaunch := int64(hongguoRandUint32()%21) + 100
	envPid := int64(hongguoRandUint32()%2000) + 10001
	env := hongguoProto([]hongguoProtoField{
		{1, envLaunch, "sint"},
		{2, int64(146331399), "sint"},
		{3, int64(146331396), "sint"},
		{5, int64(7), "sint"},
		{6, "v04.06.04.03-bugfix", "string"},
		{7, envPid, "sint"},
		{12, hongguoDeviceProto(deviceID), "message"},
		{13, hongguoProto([]hongguoProtoField{
			{1, time.Now().Unix(), "sint"},
			{2, int64(-2), "sint"},
			{4, int64(200), "sint"},
		}), "message"},
		{14, versionName, "string"},
	})
	queryHash := hongguoSM3Sum(append(append([]byte(rawQuery), bodyDigest...), []byte("none")...))
	dynValue := `{"cmr":16777216,"cmr2":16777216,"un_h":1879194040,"vpn":0,"kd":0,"fkd":3672518972,"pd":-1872573247,"dyn":"","do":0,"tk":true}`
	message := hongguoProto([]hongguoProtoField{
		{1, hongguoHexBytes("f7e85ffad7d7dc3bd62ac87057cf6118"), "bytes"},
		{2, int64(3), "sint"},
		{3, int64(messageRand), "sint"},
		{4, "8662", "string"},
		{5, deviceID, "string"},
		{6, "1588093228", "string"},
		{7, versionName, "string"},
		{8, "v04.06.04-ml-android", "string"},
		{9, int64(67503104), "sint"},
		{10, hongguoHexBytes("4001000000000000"), "bytes"},
		{12, int64(khronos), "sint"},
		{13, queryBodyTs, "bytes"},
		{14, queryDigest[:6], "bytes"},
		{15, nested, "message"},
		{16, hongguoProcessNonce, "string"},
		{17, int64(khronos), "sint"},
		{19, queryHash, "bytes"},
		{20, "none", "string"},
		{21, int64(312), "sint"},
		{23, env, "message"},
		{24, dynValue, "string"},
	})

	random := hongguoRandUint32()
	hash, seed := hongguoKeyHash(hongguoMedusaSignKey, random)
	transformed := hongguoXmxor(message, hash)
	transformed = append(hongguoHexBytes("4001000000000000"), transformed...)
	for i, j := 0, len(transformed)-1; i < j; i, j = i+1, j-1 {
		transformed[i], transformed[j] = transformed[j], transformed[i]
	}
	for i := range transformed {
		transformed[i] ^= seed[(^i)&3]
	}

	check := (uint32(queryDigest[0]&63) << 14) | 0x18000001 | (uint32(queryBodyTs[0]&63) << 8)
	packed := make([]byte, 0, 1+4+4+len(transformed)+2)
	packed = append(packed, 0x35)
	packed = append(packed, hongguoLe32(hongguoRandUint32())...)
	packed = append(packed, hongguoLe32(check)...)
	packed = append(packed, transformed...)
	packed = append(packed, byte((random>>16)&255), byte((random>>24)&255))

	aes := hongguoNewAesV3(hongguoHexBytes("f1593376766ea98d34f31b057a9d5be4"), khronos)
	encrypted := aes.encrypt(packed, hongguoHexBytes("1fe109a4125283f418de9e051a969e12"))

	version := hongguoHexBytes("03000000f7e85ffad7d7dc3bd62ac87057cf6118")
	prefix := make([]byte, 20)
	for i := 0; i < 20; i += 4 {
		binary.LittleEndian.PutUint32(prefix[i:], binary.LittleEndian.Uint32(version[i:])^khronos)
	}
	out := append(prefix, byte(random&255), byte((random>>8)&255), 0, 1)
	out = append(out, encrypted...)
	return base64.StdEncoding.EncodeToString(out)
}

// ---------- Helios ----------

func hongguoHelios(khronos uint32) string {
	random := hongguoRandUint32()
	data := append(hongguoLe32(random), []byte("8662")...)
	ascii := []byte(hex.EncodeToString(hongguoMd5Sum(data)))

	var words [4]uint64
	for i := 0; i < 4; i++ {
		words[i] = binary.LittleEndian.Uint64(ascii[i*8 : i*8+8])
	}
	table := make([]uint64, 0, 35)
	table = append(table, words[0])
	b0, b8 := words[0], words[1]
	queue := []uint64{words[2], words[3]}
	for i := 0; i < 34; i++ {
		x8 := bits.RotateLeft64(b8, -8) + b0
		x8 ^= uint64(i)
		queue = append(queue, x8)
		x8 ^= bits.RotateLeft64(b0, -61)
		table = append(table, x8)
		b0 = x8
		b8 = queue[0]
		queue = queue[1:]
	}

	text := []byte(strconv.FormatUint(uint64(khronos), 10) + "-1588093228-8662")
	paddedLength := (len(text) + 1 + 15) / 16 * 16
	pad := make([]byte, paddedLength)
	for i := range pad {
		pad[i] = byte(16 - (len(text) % 16))
	}
	copy(pad, text)

	output := make([]byte, 0, 4+paddedLength)
	output = append(output, hongguoLe32(random)...)
	for at := 0; at < len(pad); at += 16 {
		a := binary.LittleEndian.Uint64(pad[at : at+8])
		b := binary.LittleEndian.Uint64(pad[at+8 : at+16])
		for i := 0; i < 34; i++ {
			b = table[i] ^ (a + bits.RotateLeft64(b, -8))
			a = b ^ bits.RotateLeft64(a, -61)
		}
		block := make([]byte, 16)
		binary.LittleEndian.PutUint64(block[0:], a)
		binary.LittleEndian.PutUint64(block[8:], b)
		output = append(output, block...)
	}
	return base64.StdEncoding.EncodeToString(output)
}

// hongguoAppDeviceQuery 返回与官方客户端一致的设备查询参数。
func hongguoAppDeviceQuery(deviceID, installID string) url.Values {
	return url.Values{
		"ac":                      {"wifi"},
		"aid":                     {"8662"},
		"app_name":                {"novelread"},
		"channel":                 {"update_64"},
		"compliance_status":       {"0"},
		"device_brand":            {"Redmi"},
		"device_id":               {deviceID},
		"device_platform":         {"android"},
		"device_type":             {"25053RT47C"},
		"dpi":                     {"520"},
		"dragon_device_type":      {"phone"},
		"host_abi":                {"arm64-v8a"},
		"iid":                     {installID},
		"is_android_pad_screen":   {"0"},
		"language":                {"zh"},
		"manifest_version_code":   {"73532"},
		"need_personal_recommend": {"1"},
		"os":                      {"android"},
		"os_api":                  {"36"},
		"os_version":              {"16"},
		"player_so_load":          {"1"},
		"pv_player":               {"73532"},
		"resolution":              {"1280*2772"},
		"ssmix":                   {"a"},
		"update_version_code":     {"73532"},
		"version_code":            {"73532"},
		"version_name":            {"7.3.5.32"},
	}
}
