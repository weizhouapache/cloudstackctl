package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestE2E_Managed_ListAndDescribeAllKinds(t *testing.T) {
	requireManagedE2EEnabled(t)

	kinds := []string{"Application", "Component", "VirtualMachineSpec", "VirtualMachine"}
	for _, kind := range kinds {
		path := fmt.Sprintf("/list?kind=%s", kind)
		if kind == "VirtualMachine" {
			path += "&all-vms=false"
		}
		status, body := controllerRequest(t, http.MethodGet, path, nil)
		failNon200(t, status, body, "list "+kind)

		var arr []map[string]any
		if err := json.Unmarshal(body, &arr); err != nil {
			t.Fatalf("failed to decode list %s response: %v; body=%s", kind, err, string(body))
		}
		if len(arr) == 0 {
			continue
		}

		name, _ := arr[0]["name"].(string)
		if name == "" {
			if md, ok := arr[0]["metadata"].(map[string]any); ok {
				name, _ = md["name"].(string)
			}
		}
		if name == "" {
			continue
		}

		descPath := fmt.Sprintf("/describe?kind=%s&name=%s", kind, name)
		status, body = controllerRequest(t, http.MethodGet, descPath, nil)
		failNon200(t, status, body, "describe "+kind)
	}
}

func TestE2E_Managed_ApplyAndDelete_OptionalMutation(t *testing.T) {
	requireManagedE2EEnabled(t)
	if !envEnabled("E2E_ALLOW_MUTATION") {
		t.Skip("set E2E_ALLOW_MUTATION=true to run managed mutation e2e tests")
	}

	ts := time.Now().UnixNano()
	suffix := fmt.Sprintf("-%d", ts)

	fixture := strings.TrimSpace(os.Getenv("E2E_MANAGED_FIXTURE"))
	if fixture == "" {
		fixture = filepath.Join("fixtures", "application-full.yaml")
	}
	raw, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("failed to read managed e2e fixture %s: %v", fixture, err)
	}
	payload, err := rewriteManagedFixture(raw, suffix)
	if err != nil {
		t.Fatalf("failed to rewrite managed e2e fixture: %v", err)
	}

	status, body := controllerRequest(t, http.MethodPost, "/apply", payload)
	if status != http.StatusOK {
		t.Fatalf("managed apply returned %d: %s", status, string(body))
	}

	// Verify each managed kind can be described after apply.
	for _, pair := range [][2]string{
		{"VirtualMachineSpec", "frontend-vmspec" + suffix},
		{"VirtualMachineSpec", "backend-vmspec" + suffix},
		{"Component", "frontend" + suffix},
		{"Component", "backend" + suffix},
		{"Application", "example-app" + suffix},
	} {
		kind := pair[0]
		name := pair[1]
		status, body := controllerRequest(t, http.MethodGet, fmt.Sprintf("/describe?kind=%s&name=%s", kind, name), nil)
		if status != http.StatusOK {
			t.Fatalf("describe %s/%s returned %d: %s", kind, name, status, string(body))
		}
	}

	// Cleanup in reverse order.
	for _, pair := range [][2]string{
		{"Application", "example-app" + suffix},
		{"Component", "frontend" + suffix},
		{"Component", "backend" + suffix},
		{"VirtualMachineSpec", "frontend-vmspec" + suffix},
		{"VirtualMachineSpec", "backend-vmspec" + suffix},
	} {
		kind := pair[0]
		name := pair[1]
		delPayload := []byte(fmt.Sprintf(`{"kind":"%s","name":"%s"}`, kind, name))
		status, body := controllerRequest(t, http.MethodPost, "/delete", delPayload)
		if status != http.StatusOK {
			t.Fatalf("delete %s/%s returned %d: %s", kind, name, status, string(body))
		}
	}
}

func rewriteManagedFixture(raw []byte, suffix string) ([]byte, error) {
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	var docs []map[string]any
	nameMap := map[string]string{
		"frontend-vmspec": "frontend-vmspec" + suffix,
		"backend-vmspec":  "backend-vmspec" + suffix,
		"frontend":        "frontend" + suffix,
		"backend":         "backend" + suffix,
		"example-app":     "example-app" + suffix,
	}

	for {
		var doc map[string]any
		if err := dec.Decode(&doc); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		if doc == nil {
			continue
		}

		kind, _ := doc["kind"].(string)
		meta, _ := doc["metadata"].(map[string]any)
		if meta != nil {
			if name, _ := meta["name"].(string); name != "" {
				if renamed, ok := nameMap[name]; ok {
					meta["name"] = renamed
				}
			}
			doc["metadata"] = meta
		}

		spec, _ := doc["spec"].(map[string]any)
		switch kind {
		case "Component":
			if spec != nil {
				if vmspec, _ := spec["virtualMachineSpec"].(string); vmspec != "" {
					if renamed, ok := nameMap[vmspec]; ok {
						spec["virtualMachineSpec"] = renamed
					}
				}
				spec["replicas"] = 0
				doc["spec"] = spec
			}
		case "Application":
			if spec != nil {
				if comps, ok := spec["components"].([]any); ok {
					for index, component := range comps {
						compMap, ok := component.(map[string]any)
						if !ok {
							continue
						}
						if name, _ := compMap["name"].(string); name != "" {
							if renamed, ok := nameMap[name]; ok {
								compMap["name"] = renamed
							}
						}
						if dependsOn, _ := compMap["dependsOn"].(string); dependsOn != "" {
							if renamed, ok := nameMap[dependsOn]; ok {
								compMap["dependsOn"] = renamed
							}
						}
						if vmspec, _ := compMap["virtualMachineSpec"].(string); vmspec != "" {
							if renamed, ok := nameMap[vmspec]; ok {
								compMap["virtualMachineSpec"] = renamed
							}
						}
						compMap["replicas"] = 0
						comps[index] = compMap
					}
					spec["components"] = comps
				}
				doc["spec"] = spec
			}
		}

		docs = append(docs, doc)
	}

	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	for _, doc := range docs {
		if err := enc.Encode(doc); err != nil {
			return nil, err
		}
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
