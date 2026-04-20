import re

with open('pkg/orchestrator/manager.go', 'r') as f:
    content = f.read()

# 1. Rename ListServerlessContainers to ListManagedContainers and update filter
old_func = r'func \(sm \*ServiceManager\) ListServerlessContainers\(\) \[\]ContainerSummary \{[^}]+resp, err := sm\.dockerClient\.Get\(`http://localhost/containers/json\?all=true&filters=\{"name":\["minisky-serverless"\]\}`\).*?return out\n\}'

# The new function gets all containers prefixed with 'minisky-'
new_func = """func (sm *ServiceManager) ListManagedContainers() []ContainerSummary {
	resp, err := sm.dockerClient.Get(`http://localhost/containers/json?all=true&filters={"name":["minisky-"]}`)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var raw []struct {
		Names  []string `json:"Names"`
		Status string   `json:"Status"`
		Image  string   `json:"Image"`
	}
	import_json := __import__("encoding/json") # just a reminder that json is imported
	if err := __import__("encoding/json").NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil
	}
	out := make([]ContainerSummary, 0, len(raw))
	for _, c := range raw {
		name := ""
		if len(c.Names) > 0 {
			name = __import__("strings").TrimPrefix(c.Names[0], "/")
		}
		out = append(out, ContainerSummary{Name: name, Status: c.Status, Image: c.Image})
	}
	return out
}"""

# Wait, the python script replacing go code needs to be careful about imports. Let's do it cleaner.
