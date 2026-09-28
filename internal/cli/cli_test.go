package cli

import (
	"reflect"
	"testing"
	"time"
)

func TestExistingInvocations(t *testing.T) {
	for _, argv := range [][]string{
		{"-s", "flux-boot-windows-amd64-3.2.5.zip", "--oss-destination", "encoded-secret"},
		{"-s", "build/app/outputs/flutter-apk/app-release.apk", "--oss-destination", "encoded-secret"},
		{"--source", "./cyberme-svc.trigger", "--destination", "encoded-secret", "--commands", "/root/cyberme/go-service.sh"},
		{"-d", "encoded-secret", "--commands", "echo hello"},
	} {
		a, err := Parse(argv)
		if err != nil {
			t.Fatal(err)
		}
		if a.Retries != 3 || a.Timeout != 10*time.Minute {
			t.Fatalf("wrong defaults: %+v", a)
		}
	}
}

func TestMultipleValuesAndOptions(t *testing.T) {
	a, err := Parse([]string{"-s", "a", "b", "--source=c", "-d", "secret", "--precommands", "echo one", "echo two", "-c", "echo three", "-c=echo four", "--retries=0", "--retry-delay", "5s", "--timeout=20m"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.Sources, []string{"a", "b", "c"}) || len(a.Precommands) != 2 || len(a.Commands) != 2 || a.Retries != 0 || a.Timeout != 20*time.Minute {
		t.Fatalf("bad args: %+v", a)
	}
}

func TestInvalidOptions(t *testing.T) {
	for _, tail := range [][]string{{"--retries", "-1"}, {"--timeout", "0"}, {"--retry-delay", "-1s"}, {"--port=65536"}, {"--source"}, {"--unknown"}, {"--command-timeout=-1s"}} {
		if _, err := Parse(append([]string{"-d", "secret"}, tail...)); err == nil {
			t.Fatalf("accepted %v", tail)
		}
	}
	if _, err := Parse(nil); err == nil {
		t.Fatal("missing destination accepted")
	}
	if _, err := Parse([]string{"--oss-destination", "secret", "-c", "deploy"}); err == nil {
		t.Fatal("silently discarded command")
	}
}
