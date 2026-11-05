package update

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJobRoundTrip(t *testing.T) {
	in := Job{Version: "1.2.3", Repo: "o/r", Current: "1.2.2", HostPID: 42, Relaunch: "/x/app", Args: []string{"--config", "c.yaml"}, Theme: "dark"}
	path, err := WriteJob(in)
	if err != nil {
		t.Fatalf("WriteJob: %v", err)
	}
	defer os.Remove(path)
	out, err := ReadJob(path)
	if err != nil {
		t.Fatalf("ReadJob: %v", err)
	}
	if out.Version != in.Version || out.Repo != in.Repo || out.HostPID != 42 || out.Relaunch != in.Relaunch || len(out.Args) != 2 || out.Theme != "dark" {
		t.Errorf("round trip = %+v", out)
	}
}

func TestJobChannel(t *testing.T) {
	path, err := WriteJob(Job{Version: "0.4.7-dev.1.abc1234", Repo: "o/r", Relaunch: "/x/app", Channel: ChannelDev})
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
	if j, err := ReadJob(path); err != nil || j.Channel != ChannelDev {
		t.Errorf("channel round trip = %+v, %v", j, err)
	}
	// An old job file without the field reads as stable.
	old := filepath.Join(t.TempDir(), "old.json")
	os.WriteFile(old, []byte(`{"version":"1.0.0","repo":"o/r","relaunch":"/x/app"}`), 0o600)
	j, err := ReadJob(old)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := New(Config{Repo: j.Repo, Current: "0.9.0", Channel: j.Channel}); err != nil || j.Channel != "" {
		t.Errorf("old job channel = %q, err %v", j.Channel, err)
	}
}

func TestReadJobRejectsIncomplete(t *testing.T) {
	p := filepath.Join(t.TempDir(), "job.json")
	os.WriteFile(p, []byte(`{"version":"1.0.0"}`), 0o600)
	if _, err := ReadJob(p); err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("err = %v, want incomplete", err)
	}
}

func TestProgressReader(t *testing.T) {
	var last, total int64
	pr := &progressReader{r: strings.NewReader(strings.Repeat("x", 1000)), total: 1000, fn: func(d, tot int64) { last, total = d, tot }}
	buf := make([]byte, 300)
	for {
		if _, err := pr.Read(buf); err != nil {
			break
		}
	}
	if last != 1000 || total != 1000 {
		t.Errorf("progress = %d/%d", last, total)
	}
}

func TestWaitExitGoneProcess(t *testing.T) {
	if !WaitExit(0, 0) {
		t.Error("pid 0 should count as gone")
	}
}
