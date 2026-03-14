package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"text/tabwriter"

	"cloudstackctl/pkg/cloudstack"
)

// ListTemplates prints a table of templates.
func ListTemplates(name string) error {
	client, err := cloudstack.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	params := client.Template.NewListTemplatesParams("")
	if name != "" {
		params.SetName(name)
	}
	resp, err := client.Template.ListTemplates(params)
	if err != nil {
		return fmt.Errorf("cloudstack API error: %w", err)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tID\tOS\tFEATURED")
	for _, t := range resp.Templates {
		fmt.Fprintf(w, "%s\t%s\t%s\t%t\n", t.Name, t.Id, t.Ostypename, t.Isfeatured)
	}
	w.Flush()
	return nil
}

// DescribeTemplate prints JSON for a template by name.
func DescribeTemplate(name string) error {
	client, err := cloudstack.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	params := client.Template.NewListTemplatesParams("")
	params.SetName(name)
	resp, err := client.Template.ListTemplates(params)
	if err != nil {
		return fmt.Errorf("cloudstack API error: %w", err)
	}
	if resp == nil || len(resp.Templates) == 0 {
		return fmt.Errorf("template %s not found", name)
	}
	data, _ := json.MarshalIndent(resp.Templates[0], "", "  ")
	log.Println(string(data))
	return nil
}

// DeleteTemplate deletes a template by name.
func DeleteTemplate(name string) error {
	client, err := cloudstack.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	params := client.Template.NewListTemplatesParams("")
	params.SetName(name)
	resp, err := client.Template.ListTemplates(params)
	if err != nil {
		return fmt.Errorf("cloudstack API error: %w", err)
	}
	if resp == nil || len(resp.Templates) == 0 {
		return fmt.Errorf("template %s not found", name)
	}
	tid := resp.Templates[0].Id
	dp := client.Template.NewDeleteTemplateParams(tid)
	if _, err := client.Template.DeleteTemplate(dp); err != nil {
		return fmt.Errorf("failed to delete template %s: %w", name, err)
	}
	log.Printf("Template %s deleted from CloudStack (id=%s)", name, tid)
	return nil
}

// ResolveTemplate returns the CloudStack template ID for a given template name.
func ResolveTemplate(name string) (string, error) {
	// If the value looks like a UUID, treat it as an ID and return it.
	if IsUUID(name) {
		return name, nil
	}

	client, err := cloudstack.NewClient()
	if err != nil {
		return "", fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	params := client.Template.NewListTemplatesParams("")
	params.SetName(name)
	params.SetTemplatefilter("all") // Search all templates (not just "featured" or "self") to allow resolving by name regardless of ownership or featured status.
	resp, err := client.Template.ListTemplates(params)
	if err != nil {
		return "", fmt.Errorf("cloudstack API error: %w", err)
	}
	if resp == nil || len(resp.Templates) == 0 {
		return "", fmt.Errorf("template %s not found", name)
	}
	return resp.Templates[0].Id, nil
}
