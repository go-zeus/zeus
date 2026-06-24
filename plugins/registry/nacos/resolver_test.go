package nacos

import (
	"testing"
)

// TestParseURL_SingleServer 单 server URL 解析
func TestParseURL_SingleServer(t *testing.T) {
	opts := parseURLOptions("nacos://127.0.0.1:8848")
	if len(opts) == 0 {
		t.Fatal("expected non-empty options")
	}
	r := &nacosRegistry{}
	for _, opt := range opts {
		opt(r)
	}
	if len(r.servers) != 1 {
		t.Fatalf("servers count = %d, want 1", len(r.servers))
	}
	if r.servers[0].IpAddr != "127.0.0.1" || r.servers[0].Port != 8848 {
		t.Errorf("servers[0] = %v:%v, want 127.0.0.1:8848", r.servers[0].IpAddr, r.servers[0].Port)
	}
}

// TestParseURL_MultiServer 多 server（逗号分隔）
func TestParseURL_MultiServer(t *testing.T) {
	opts := parseURLOptions("nacos://h1:8848,h2:8848,h3:8848")
	r := &nacosRegistry{}
	for _, opt := range opts {
		opt(r)
	}
	if len(r.servers) != 3 {
		t.Errorf("servers count = %d, want 3", len(r.servers))
	}
}

// TestParseURL_QueryParams query 参数解析（namespace/group）
func TestParseURL_QueryParams(t *testing.T) {
	opts := parseURLOptions("nacos://127.0.0.1:8848?namespace=prod&group=BIZ_GROUP")
	r := &nacosRegistry{}
	for _, opt := range opts {
		opt(r)
	}
	if r.namespace != "prod" {
		t.Errorf("namespace = %q, want prod", r.namespace)
	}
	if r.group != "BIZ_GROUP" {
		t.Errorf("group = %q, want BIZ_GROUP", r.group)
	}
}

// TestParseURL_AKSK query 参数 AK/SK
func TestParseURL_AKSK(t *testing.T) {
	opts := parseURLOptions("nacos://127.0.0.1:8848?ak=ACCESS_KEY&sk=SECRET_KEY")
	r := &nacosRegistry{}
	for _, opt := range opts {
		opt(r)
	}
	if r.accessKey != "ACCESS_KEY" {
		t.Errorf("accessKey = %q, want ACCESS_KEY", r.accessKey)
	}
	if r.secretKey != "SECRET_KEY" {
		t.Errorf("secretKey = %q, want SECRET_KEY", r.secretKey)
	}
}

// TestParseURL_Credentials URL 中嵌入用户名密码
func TestParseURL_Credentials(t *testing.T) {
	opts := parseURLOptions("nacos://nacos:nacospwd@127.0.0.1:8848")
	r := &nacosRegistry{}
	for _, opt := range opts {
		opt(r)
	}
	if r.username != "nacos" {
		t.Errorf("username = %q, want nacos", r.username)
	}
	if r.password != "nacospwd" {
		t.Errorf("password = %q, want nacospwd", r.password)
	}
	if len(r.servers) != 1 || r.servers[0].IpAddr != "127.0.0.1" {
		t.Errorf("servers = %v, want 127.0.0.1:8848", r.servers)
	}
}

// TestParseURL_DefaultPort 缺省端口补默认值 8848
func TestParseURL_DefaultPort(t *testing.T) {
	opts := parseURLOptions("nacos://127.0.0.1")
	r := &nacosRegistry{}
	for _, opt := range opts {
		opt(r)
	}
	if len(r.servers) != 1 {
		t.Fatalf("servers = %v, want 1 item", r.servers)
	}
	if r.servers[0].Port != DefaultServerPort {
		t.Errorf("port = %v, want default %v", r.servers[0].Port, DefaultServerPort)
	}
}

// TestParseURL_InvalidPort 非法端口回退到默认值
func TestParseURL_InvalidPort(t *testing.T) {
	opts := parseURLOptions("nacos://127.0.0.1:notaport")
	r := &nacosRegistry{}
	for _, opt := range opts {
		opt(r)
	}
	if len(r.servers) != 1 {
		t.Fatalf("servers = %v, want 1 item", r.servers)
	}
	if r.servers[0].Port != DefaultServerPort {
		t.Errorf("port = %v, want default %v (fallback on invalid port)", r.servers[0].Port, DefaultServerPort)
	}
}

// TestParseURL_NonNacosScheme 非 nacos:// URL 透传
func TestParseURL_NonNacosScheme(t *testing.T) {
	opts := parseURLOptions("127.0.0.1:8848")
	r := &nacosRegistry{}
	for _, opt := range opts {
		opt(r)
	}
	if len(r.servers) != 1 {
		t.Errorf("servers = %v, want 1", r.servers)
	}
}

// TestParseURL_UnknownQuery 未知 query 参数静默忽略
func TestParseURL_UnknownQuery(t *testing.T) {
	opts := parseURLOptions("nacos://127.0.0.1:8848?unknown=value&foo=bar")
	r := &nacosRegistry{}
	for _, opt := range opts {
		opt(r)
	}
	// 不会 panic，server 正常解析
	if len(r.servers) != 1 {
		t.Errorf("servers = %v, want 1", r.servers)
	}
}

// TestSplitAndTrim 字符串分割
func TestSplitAndTrim(t *testing.T) {
	cases := []struct {
		in, sep string
		want    []string
	}{
		{"a,b,c", ",", []string{"a", "b", "c"}},
		{" a , b , c ", ",", []string{"a", "b", "c"}},
		{"a,,b", ",", []string{"a", "b"}},
		{"", ",", nil},
		{"single", ",", []string{"single"}},
	}
	for _, tc := range cases {
		got := splitAndTrim(tc.in, tc.sep)
		if len(got) != len(tc.want) {
			t.Errorf("splitAndTrim(%q,%q) = %v, want %v", tc.in, tc.sep, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("splitAndTrim(%q,%q)[%d] = %q, want %q", tc.in, tc.sep, i, got[i], tc.want[i])
			}
		}
	}
}

// TestSplitHostPort host:port 解析
func TestSplitHostPort(t *testing.T) {
	cases := []struct {
		in       string
		defPort  int
		wantHost string
		wantPort int
	}{
		{"127.0.0.1:8848", 8848, "127.0.0.1", 8848},
		{"127.0.0.1", 8848, "127.0.0.1", 8848},
		{"host:8848", 8848, "host", 8848},
		{"host", 8848, "host", 8848},
		{"", 8848, "", 0},
	}
	for _, tc := range cases {
		host, port := splitHostPort(tc.in, tc.defPort)
		if host != tc.wantHost || port != tc.wantPort {
			t.Errorf("splitHostPort(%q,%d) = (%q,%d), want (%q,%d)",
				tc.in, tc.defPort, host, port, tc.wantHost, tc.wantPort)
		}
	}
}

// TestSimpleAtoi 整数解析
func TestSimpleAtoi(t *testing.T) {
	cases := []struct {
		in    string
		want  int
		fails bool
	}{
		{"8848", 8848, false},
		{"0", 0, false},
		{"99999", 99999, false},
		{"", 0, true},
		{"abc", 0, true},
		{"12a", 0, true},
	}
	for _, tc := range cases {
		got, err := simpleAtoi(tc.in)
		if tc.fails {
			if err == nil {
				t.Errorf("simpleAtoi(%q) want error, got nil", tc.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("simpleAtoi(%q) unexpected err: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("simpleAtoi(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// TestDefaultConstants 默认常量合理
func TestDefaultConstants(t *testing.T) {
	if DefaultServerPort != 8848 {
		t.Errorf("DefaultServerPort = %v, want 8848", DefaultServerPort)
	}
	if DefaultGroup != "DEFAULT_GROUP" {
		t.Errorf("DefaultGroup = %q, want DEFAULT_GROUP", DefaultGroup)
	}
	if DefaultClusterName != "DEFAULT" {
		t.Errorf("DefaultClusterName = %q, want DEFAULT", DefaultClusterName)
	}
}

// TestNacosRegistry_ImplementsInterfaces 编译期检查接口实现
func TestNacosRegistry_ImplementsInterfaces(t *testing.T) {
	r := New()
	if r == nil {
		t.Fatal("New returned nil")
	}
	// 编译期通过 _ = ... 检查接口
	_ = r
}
