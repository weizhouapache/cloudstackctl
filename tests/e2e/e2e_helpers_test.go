package e2e_test

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"net/url"
	"strings"
	"testing"
	"time"

	cloudstackpkg "cloudstackctl/pkg/cloudstack"
)

func envEnabled(key string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	return v == "1" || v == "true" || v == "yes"
}

func requireUnmanagedE2EEnabled(t *testing.T) {
	t.Helper()
	if !envEnabled("E2E_CLOUDSTACK") {
		t.Skip("set E2E_CLOUDSTACK=true to run real unmanaged CloudStack e2e tests")
	}
	if cfg := strings.TrimSpace(os.Getenv("E2E_CLOUDSTACK_CONFIG")); cfg != "" {
		cloudstackpkg.SetConfigFile(cfg)
	} else if p := findRepoConfig(".env.cloudstack"); p != "" {
		cloudstackpkg.SetConfigFile(p)
	}
	if _, err := cloudstackpkg.NewClient(); err != nil {
		if _, ok := err.(*cloudstackpkg.ConfigError); ok {
			t.Skipf("skipping unmanaged e2e: CloudStack credentials are not configured: %v", err)
		}
		t.Fatalf("failed to initialize CloudStack client for e2e: %v", err)
	}
	if err := preflightCloudStackEndpoint(); err != nil {
		t.Skipf("skipping unmanaged e2e: CloudStack endpoint is unreachable: %v", err)
	}
	t.Cleanup(func() { cloudstackpkg.SetConfigFile("") })
}

func findRepoConfig(name string) string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	cur := wd
	for {
		candidate := filepath.Join(cur, name)
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return candidate
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return ""
}

func preflightCloudStackEndpoint() error {
	ep := strings.TrimSpace(os.Getenv("CLOUDSTACK_ENDPOINT"))
	if ep == "" {
		// Endpoint may come from external config handling; best-effort only.
		return nil
	}
	u, err := url.Parse(ep)
	if err != nil {
		return err
	}
	host := u.Host
	if !strings.Contains(host, ":") {
		if u.Scheme == "https" {
			host += ":443"
		} else {
			host += ":80"
		}
	}
	conn, err := net.DialTimeout("tcp", host, 3*time.Second)
	if err != nil {
		return err
	}
	_ = conn.Close()
	return nil
}

func requireManagedE2EEnabled(t *testing.T) {
	t.Helper()
	if !envEnabled("E2E_MANAGED") {
		t.Skip("set E2E_MANAGED=true to run managed-resource e2e tests against controller endpoint")
	}
	status, body := controllerRequestNoFail(http.MethodGet, "/health", nil)
	if status == 0 {
		t.Skip("skipping managed e2e: controller endpoint is unreachable")
	}
	if status != http.StatusOK {
		t.Skipf("skipping managed e2e: controller health returned %d: %s", status, string(body))
	}
}

func controllerEndpoint() string {
	ep := strings.TrimSpace(os.Getenv("E2E_CONTROLLER_ENDPOINT"))
	if ep == "" {
		ep = "http://localhost:65426"
	}
	return strings.TrimRight(ep, "/")
}

func controllerRequest(t *testing.T, method, path string, body []byte) (int, []byte) {
	t.Helper()
	status, b, err := controllerRequestWithError(method, path, body)
	if err != nil {
		t.Fatalf("request %s %s failed: %v", method, controllerEndpoint()+path, err)
	}
	return status, b
}

func controllerRequestNoFail(method, path string, body []byte) (int, []byte) {
	status, b, err := controllerRequestWithError(method, path, body)
	if err != nil {
		return 0, nil
	}
	return status, b
}

func controllerRequestWithError(method, path string, body []byte) (int, []byte, error) {
	url := controllerEndpoint() + path
	var r io.Reader
	if len(body) > 0 {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, r)
	if err != nil {
		return 0, nil, err
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b, nil
}

func failNon200(t *testing.T, status int, body []byte, action string) {
	t.Helper()
	if status != http.StatusOK {
		t.Fatalf("%s returned %d: %s", action, status, string(body))
	}
}

func debugf(format string, args ...any) {
	_ = fmt.Sprintf(format, args...)
}
