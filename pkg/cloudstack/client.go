package cloudstack

import (
	"bufio"
	"os"
	"strings"

	"github.com/apache/cloudstack-go/v2/cloudstack"
)

// configFile is an optional path to a CloudStack config file
var configFile string

// SetConfigFile sets the path to the CloudStack config file to use
func SetConfigFile(path string) {
	configFile = path
}

// fileConfig is intentionally simple and matches KEY=VALUE files
type fileConfig struct {
	Endpoint  string
	APIKey    string
	SecretKey string
	VerifySSL *bool
}

// NewClient creates a new CloudStack API client. It first tries to load
// credentials from the configured config file (if set), otherwise falls
// back to reading canonical environment variables. Returns an error when
// credentials are not present.
func NewClient() (*cloudstack.CloudStackClient, error) {
	var endpoint, apiKey, secretKey string
	var verifySSL bool = true

	// If no config file explicitly set, look for default `.env.cloudstack`
	if configFile == "" {
		if _, err := os.Stat(".env.cloudstack"); err == nil {
			configFile = ".env.cloudstack"
		}
	}

	// Try config file first (simple KEY=VALUE parsing)
	if configFile != "" {
		if f, err := os.Open(configFile); err == nil {
			defer f.Close()
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				parts := strings.SplitN(line, "=", 2)
				if len(parts) != 2 {
					continue
				}
				key := strings.TrimSpace(parts[0])
				val := strings.TrimSpace(parts[1])
				switch key {
				case "CLOUDSTACK_ENDPOINT":
					endpoint = val
				case "CLOUDSTACK_API_KEY":
					apiKey = val
				case "CLOUDSTACK_SECRET_KEY":
					secretKey = val
				case "VERIFY_SSL":
					if strings.EqualFold(val, "false") || strings.EqualFold(val, "0") {
						b := false
						fc := b
						verifySSL = fc
					}
				}
			}
		}
	}

	// Fallback to environment variables for any missing values
	if endpoint == "" {
		endpoint = os.Getenv("CLOUDSTACK_ENDPOINT")
	}
	if apiKey == "" {
		apiKey = os.Getenv("CLOUDSTACK_API_KEY")
	}
	if secretKey == "" {
		secretKey = os.Getenv("CLOUDSTACK_SECRET_KEY")
	}

	// determine verify-ssl using environment if not set by config file
	if !verifySSL {
		// already set false by config file
	} else {
		vs := os.Getenv("VERIFY_SSL")
		if strings.EqualFold(vs, "false") || strings.EqualFold(vs, "0") {
			verifySSL = false
		}
	}

	if endpoint == "" || apiKey == "" || secretKey == "" {
		return nil, &ConfigError{"Missing CloudStack credentials: set CLOUDSTACK_ENDPOINT, CLOUDSTACK_API_KEY, and CLOUDSTACK_SECRET_KEY or provide a config file"}
	}

	client := cloudstack.NewAsyncClient(endpoint, apiKey, secretKey, verifySSL)
	return client, nil
}

// ConfigError is returned when client configuration is invalid
type ConfigError struct {
	Msg string
}

func (e *ConfigError) Error() string { return e.Msg }

// GetVMState retrieves the current state of a VM from CloudStack
func GetVMState(client *cloudstack.CloudStackClient, vmID string) (string, error) {
	params := client.VirtualMachine.NewListVirtualMachinesParams()
	params.SetId(vmID)
	resp, err := client.VirtualMachine.ListVirtualMachines(params)
	if err != nil {
		return "", err
	}
	if resp == nil || len(resp.VirtualMachines) == 0 {
		return "", nil
	}
	return resp.VirtualMachines[0].State, nil
}
