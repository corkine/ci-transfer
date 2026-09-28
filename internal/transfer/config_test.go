package transfer

import (
	"encoding/base64"
	"testing"
)

func TestConfigurationCompatibility(t *testing.T) {
	value := `{"oss_bucket":"cmbed","oss_endpoint":"oss-cn-shenzhen.aliyuncs.com","key_id":"id","key_secret":"secret","path":"/service/flux-boot/","override_existing":false}`
	for _, input := range []string{value, base64.StdEncoding.EncodeToString([]byte(value))} {
		c, err := parseOSS(input)
		if err != nil {
			t.Fatal(err)
		}
		if c.Endpoint != "https://oss-cn-shenzhen.aliyuncs.com" || c.Region != "cn-shenzhen" || c.OverrideExisting == nil || *c.OverrideExisting {
			t.Fatalf("bad config: %+v", c)
		}
	}
	for _, input := range []string{"user:p:a@ss@host:/root/upload/", base64.StdEncoding.EncodeToString([]byte("user:p:a@ss@host:/root/upload/"))} {
		c, err := parseSSH(input)
		if err != nil {
			t.Fatal(err)
		}
		if c.Username != "user" || c.Password != "p:a@ss" || c.Host != "host" || c.Path != "/root/upload/" {
			t.Fatalf("bad config: %+v", c)
		}
	}
	if c, err := parseSSH("u:p@[::1]:/tmp/"); err != nil || c.Host != "::1" {
		t.Fatalf("IPv6: %+v %v", c, err)
	}
	command := "printf '%s\\n' \"$(uname)\""
	if decodeValue(base64.StdEncoding.EncodeToString([]byte(command))) != command {
		t.Fatal("command decoding changed")
	}
	if got := shellQuote("echo 'hello' && echo $HOME"); got != "'echo '\"'\"'hello'\"'\"' && echo $HOME'" {
		t.Fatalf("incorrect shell quoting: %s", got)
	}
}
