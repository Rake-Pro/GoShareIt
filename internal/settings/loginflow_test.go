package settings

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/Rake-Pro/GoShareIt/internal/core/config"
)

// fakeNextcloud serves the Login Flow v2 endpoints: pending for `pendingPolls`
// polls, then success.
func fakeNextcloud(t *testing.T, pendingPolls int32) *httptest.Server {
	t.Helper()
	var polls int32
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/index.php/login/v2", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		fmt.Fprintf(w, `{"poll":{"token":"tok123","endpoint":%q},"login":%q}`,
			srv.URL+"/login/v2/poll", srv.URL+"/login/v2/flow")
	})
	mux.HandleFunc("/login/v2/poll", func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("token") != "tok123" {
			http.Error(w, "bad token", http.StatusBadRequest)
			return
		}
		if atomic.AddInt32(&polls, 1) <= pendingPolls {
			http.Error(w, "pending", http.StatusNotFound)
			return
		}
		fmt.Fprintf(w, `{"server":%q,"loginName":"greg","appPassword":"minted-app-pw"}`, srv.URL)
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestBrowserLoginMintsAndStoresPassword(t *testing.T) {
	testHome(t)
	srv := fakeNextcloud(t, 1)

	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	var opened string
	svc := &Service{
		ConfigPath: cfgPath,
		OpenURL:    func(u string) error { opened = u; return nil },
	}

	login, err := svc.BrowserLogin(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if login.LoginName != "greg" {
		t.Errorf("login name = %q", login.LoginName)
	}
	if opened != srv.URL+"/login/v2/flow" {
		t.Errorf("browser opened %q", opened)
	}

	// Nothing persists until Save: the credential rides SaveRequest.NewPassword.
	res, err := svc.Load()
	if err != nil {
		t.Fatal(err)
	}
	if res.HasPassword {
		t.Fatal("password must not be stored before Save")
	}
	cfg := res.Config
	cfg.Nextcloud.BaseURL = srv.URL
	cfg.Nextcloud.Username = login.LoginName
	if err := svc.Save(&SaveRequest{Config: cfg, NewPassword: login.AppPassword}); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Password() != "minted-app-pw" {
		t.Errorf("password = %q", loaded.Password())
	}
}

func TestBrowserLoginRejectsBadURL(t *testing.T) {
	testHome(t)
	svc := &Service{ConfigPath: "unused"}
	if _, err := svc.BrowserLogin("cloud.example.com"); err == nil {
		t.Fatal("expected error for URL without scheme")
	}
	// The flow returns a minted app password, so it must not run over cleartext
	// http to a remote host.
	if _, err := svc.BrowserLogin("http://cloud.example.com"); err == nil {
		t.Fatal("expected error for plain http server URL")
	}
}

func TestResetDefaults(t *testing.T) {
	testHome(t)
	svc := &Service{ConfigPath: filepath.Join(t.TempDir(), "config.yaml"), Version: "dev"}
	res, err := svc.ResetDefaults()
	if err != nil {
		t.Fatal(err)
	}
	if res.Config.Hotkeys.Region == "" {
		t.Error("factory defaults missing hotkeys")
	}
	if res.Config.Upload.FilenameTemplate == "" {
		t.Error("factory defaults missing filename template")
	}
	if res.Config.Nextcloud.PasswordFile == "" {
		t.Error("factory defaults missing password_file")
	}
}
