package handlers

import "fmt"

type projectScopedParams interface {
	SetProjectid(string)
}

type listAllFlagParams interface {
	SetListall(bool)
}

func resolveProjectID(project string) (string, error) {
	if project == "" {
		return "", nil
	}
	pid, err := ResolveProject(project)
	if err == nil {
		return pid, nil
	}
	if IsUUID(project) {
		return project, nil
	}
	return "", fmt.Errorf("failed to resolve project %s: %w", project, err)
}

func setProjectOnParams(params projectScopedParams, project string) error {
	if project == "" {
		return nil
	}
	pid, err := resolveProjectID(project)
	if err != nil {
		return err
	}
	params.SetProjectid(pid)
	return nil
}

// SetProjectOnParams resolves and sets the project ID on CloudStack params.
func SetProjectOnParams(params projectScopedParams, project string) error {
	return setProjectOnParams(params, project)
}

// setListAllOnParams sets projectid=-1 on the params, which instructs CloudStack
// to return resources from all projects (and non-project resources) in one call.
func setListAllOnParams(params projectScopedParams, allProjects bool) {
	if allProjects {
		params.SetProjectid("-1")
		if p, ok := any(params).(listAllFlagParams); ok {
			p.SetListall(true)
		}
	}
}

// SetListAllOnParams enables CloudStack all-projects listing on supported params.
func SetListAllOnParams(params projectScopedParams, allProjects bool) {
	setListAllOnParams(params, allProjects)
}
