package transfer

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"
)

// Preserve the Rust implementation's plain text / nested standard Base64 input.
func decodeValue(value string) string {
	for range 16 {
		decoded, err := base64.StdEncoding.DecodeString(value)
		if err != nil || !utf8.Valid(decoded) || len(decoded) == 0 || string(decoded) == value {
			break
		}
		value = string(decoded)
	}
	return value
}

type ossConfig struct {
	Bucket           string `json:"oss_bucket"`
	Endpoint         string `json:"oss_endpoint"`
	KeySecret        string `json:"key_secret"`
	KeyID            string `json:"key_id"`
	Path             string `json:"path"`
	Region           string `json:"oss_region"`
	OverrideExisting *bool  `json:"override_existing"` // Accepted for compatibility; old implementation ignored it.
}

func parseOSS(value string) (ossConfig, error) {
	var c ossConfig
	if err := json.Unmarshal([]byte(decodeValue(value)), &c); err != nil {
		return c, fmt.Errorf("invalid OSS JSON configuration")
	}
	if c.Bucket == "" || c.Endpoint == "" || c.KeyID == "" || c.KeySecret == "" || c.Path == "" {
		return c, fmt.Errorf("OSS configuration requires oss_bucket, oss_endpoint, key_id, key_secret and path")
	}
	if !strings.Contains(c.Endpoint, "://") {
		c.Endpoint = "https://" + c.Endpoint
	}
	u, err := url.Parse(c.Endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return c, fmt.Errorf("invalid OSS endpoint: expected an HTTP(S) host without credentials or path")
	}
	c.Endpoint = strings.TrimSuffix(c.Endpoint, "/")
	if c.Region == "" {
		host := strings.TrimSuffix(u.Hostname(), ".aliyuncs.com")
		if host != u.Hostname() && strings.HasPrefix(host, "oss-") && !strings.HasPrefix(host, "oss-accelerate") {
			c.Region = strings.TrimSuffix(strings.TrimPrefix(host, "oss-"), "-internal")
		}
	}
	return c, nil
}

type sshConfig struct{ Username, Password, Host, Path string }

func parseSSH(value string) (sshConfig, error) {
	var c sshConfig
	value = decodeValue(value)
	at := strings.LastIndex(value, "@")
	if at < 0 {
		return c, fmt.Errorf("invalid SSH destination: expected user:password@host:/path")
	}
	var ok bool
	c.Username, c.Password, ok = strings.Cut(value[:at], ":")
	if !ok || c.Username == "" {
		return c, fmt.Errorf("invalid SSH credentials format")
	}
	server := value[at+1:]
	if strings.HasPrefix(server, "[") {
		end := strings.Index(server, "]:")
		if end < 0 {
			return c, fmt.Errorf("invalid bracketed SSH host")
		}
		c.Host, c.Path = server[1:end], server[end+2:]
	} else {
		c.Host, c.Path, ok = strings.Cut(server, ":")
		if !ok {
			return c, fmt.Errorf("SSH destination requires a remote path")
		}
	}
	if c.Host == "" || c.Path == "" {
		return c, fmt.Errorf("SSH host and remote path must not be empty")
	}
	return c, nil
}
