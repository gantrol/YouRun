package discovery

import (
	"bytes"
	"io"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// A Flutter SDK dependency also occurs in packages and plugins. A runnable
// application additionally needs an entry file and a checked-in platform host.
func (s *scan) flutterCommands(path string, data []byte, rule Rule) error {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return err
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		s.issue(path, "scanPartial")
		return nil
	}
	count := 0
	if !staticYAML(&document, 0, &count) {
		s.issue(path, "scanPartial")
		return nil
	}
	var values map[string]any
	if err := document.Decode(&values); err != nil {
		return err
	}
	if fieldString(values, []string{"dependencies", "flutter", "sdk"}) != "flutter" {
		return nil
	}
	dir := filepath.Dir(path)
	platforms := []string{}
	for platform, evidence := range rule.Options.PlatformFiles {
		for _, file := range evidence {
			candidate := localJoin(dir, file)
			if candidate != "" && s.entries[candidate] && len(s.read(candidate)) > 0 {
				platforms = append(platforms, platform)
				break
			}
		}
	}
	if len(platforms) == 0 {
		return nil
	}
	sort.Strings(platforms)
	for _, entry := range rule.Options.EntryFiles {
		candidate := localJoin(dir, entry)
		if candidate == "" || !s.entries[candidate] || len(s.read(candidate)) == 0 {
			continue
		}
		variables := map[string]string{"entry": filepath.ToSlash(entry)}
		// Flutter selects an attached device or prompts when several are found.
		// Discovery does not enumerate devices or assume one is available.
		s.addRuleCommand(rule, "run", path, dir, variables, "", true)
		for _, platform := range platforms {
			s.addRuleCommand(rule, "build-"+platform, path, dir, variables, "", true)
		}
	}
	return nil
}
