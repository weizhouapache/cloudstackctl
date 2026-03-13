package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	v1 "cloudstackctl/apis/v1"

	"github.com/spf13/cobra"
)

// reconcileCmd triggers ad-hoc reconciliation via controller HTTP API
var reconcileCmd = &cobra.Command{
	Use:   "reconcile <resource-type> <name>",
	Short: "Trigger on-demand reconciliation for a resource (controller mode)",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		if standalone {
			log.Fatal("reconcile command requires controller mode (not standalone)")
		}
		kind := args[0]
		name := args[1]

		payload := map[string]string{"kind": kind, "name": name}
		b, _ := json.Marshal(payload)

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Post("http://localhost:65426/reconcile", "application/json", bytes.NewReader(b))
		if err != nil {
			log.Fatalf("failed to call controller: %v", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode/100 != 2 {
			log.Fatalf("controller returned error: %s", string(body))
		}

		// If --wait flag set, poll /status until resource is ready or drift resolved
		wait, _ := cmd.Flags().GetBool("wait")
		timeoutSec, _ := cmd.Flags().GetInt("timeout")
		intervalSec, _ := cmd.Flags().GetInt("interval")
		fmt.Println(string(body))
		if wait {
			deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
			for time.Now().Before(deadline) {
				statusURL := fmt.Sprintf("http://localhost:65426/status?kind=%s&name=%s", kind, name)
				sresp, err := client.Get(statusURL)
				if err != nil {
					log.Printf("status check failed: %v", err)
					time.Sleep(time.Duration(intervalSec) * time.Second)
					continue
				}
				body, _ := io.ReadAll(sresp.Body)
				sresp.Body.Close()
				if sresp.StatusCode/100 != 2 {
					log.Printf("status endpoint returned: %s", string(body))
					time.Sleep(time.Duration(intervalSec) * time.Second)
					continue
				}

				// Parse and evaluate readiness
				if kind == "Component" {
					var comp v1.Component
					if err := json.Unmarshal(body, &comp); err != nil {
						log.Printf("failed to parse component status: %v", err)
						time.Sleep(time.Duration(intervalSec) * time.Second)
						continue
					}
					if comp.Status.Ready {
						fmt.Printf("component %s is ready\n", name)
						return
					}
				} else if kind == "Application" {
					var app v1.Application
					if err := json.Unmarshal(body, &app); err != nil {
						log.Printf("failed to parse application status: %v", err)
						time.Sleep(time.Duration(intervalSec) * time.Second)
						continue
					}
					if app.Status.Ready {
						fmt.Printf("application %s is ready\n", name)
						return
					}
				} else if kind == "VirtualMachine" {
					var vm v1.VirtualMachine
					if err := json.Unmarshal(body, &vm); err != nil {
						log.Printf("failed to parse VM status: %v", err)
						time.Sleep(time.Duration(intervalSec) * time.Second)
						continue
					}
					// consider VM reconciled when not in drift and observed state present
					if !vm.Status.Drift && vm.Status.ObservedState != "" {
						fmt.Printf("virtualmachine %s reconciled (state=%s)\n", name, vm.Status.ObservedState)
						return
					}
				}

				time.Sleep(time.Duration(intervalSec) * time.Second)
			}
			log.Fatalf("timeout waiting for reconciliation to complete (timeout=%ds)", timeoutSec)
		}
	},
}

func init() {
	reconcileCmd.Flags().Bool("wait", false, "wait until reconciliation completes")
	reconcileCmd.Flags().Int("timeout", 120, "timeout in seconds when waiting")
	reconcileCmd.Flags().Int("interval", 2, "poll interval in seconds when waiting")
	rootCmd.AddCommand(reconcileCmd)
}
