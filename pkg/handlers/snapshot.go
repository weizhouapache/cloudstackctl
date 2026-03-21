package handlers

import (
	"fmt"
	"log"
	"os"
	"text/tabwriter"

	"cloudstackctl/pkg/cloudstack"
)

// ListSnapshots prints a table of snapshots.
func ListSnapshots(name string) error {
	client, err := cloudstack.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	params := client.Snapshot.NewListSnapshotsParams()
	if name != "" {
		params.SetName(name)
	}
	resp, err := client.Snapshot.ListSnapshots(params)
	if err != nil {
		return fmt.Errorf("cloudstack API error: %w", err)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tID\tVOLUME ID\tSTATE")
	for _, s := range resp.Snapshots {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", s.Name, s.Id, s.Volumeid, s.State)
	}
	w.Flush()
	return nil
}

// DescribeSnapshot returns the snapshot object from CloudStack by name.
func DescribeSnapshot(name string) (any, error) {
	client, err := cloudstack.NewClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	params := client.Snapshot.NewListSnapshotsParams()
	params.SetName(name)
	resp, err := client.Snapshot.ListSnapshots(params)
	if err != nil {
		return nil, fmt.Errorf("cloudstack API error: %w", err)
	}
	if resp == nil || len(resp.Snapshots) == 0 {
		return nil, fmt.Errorf("snapshot %s not found", name)
	}
	return resp.Snapshots[0], nil
}

// DeleteSnapshot deletes a snapshot by name.
func DeleteSnapshot(name string) (string, error) {
	client, err := cloudstack.NewClient()
	if err != nil {
		return "", fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	params := client.Snapshot.NewListSnapshotsParams()
	params.SetName(name)
	resp, err := client.Snapshot.ListSnapshots(params)
	if err != nil {
		return "", fmt.Errorf("cloudstack API error: %w", err)
	}
	if resp == nil || len(resp.Snapshots) == 0 {
		return "", fmt.Errorf("snapshot %s not found", name)
	}
	sid := resp.Snapshots[0].Id
	dp := client.Snapshot.NewDeleteSnapshotParams(sid)
	if _, err := client.Snapshot.DeleteSnapshot(dp); err != nil {
		return "", fmt.Errorf("failed to delete snapshot %s: %w", name, err)
	}
	log.Printf("Snapshot %s deleted from CloudStack (id=%s)", name, sid)
	return sid, nil
}

// ResolveSnapshot returns the CloudStack snapshot ID for a given snapshot name.
func ResolveSnapshot(name string) (string, error) {
	// If the value looks like a UUID, treat it as an ID and return it.
	if IsUUID(name) {
		return name, nil
	}

	client, err := cloudstack.NewClient()
	if err != nil {
		return "", fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	params := client.Snapshot.NewListSnapshotsParams()
	params.SetName(name)
	resp, err := client.Snapshot.ListSnapshots(params)
	if err != nil {
		return "", fmt.Errorf("cloudstack API error: %w", err)
	}
	if resp == nil || len(resp.Snapshots) == 0 {
		return "", fmt.Errorf("snapshot %s not found", name)
	}
	return resp.Snapshots[0].Id, nil
}
