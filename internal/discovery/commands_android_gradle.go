package discovery

import (
	"bytes"
	"encoding/xml"
	"io"
	"path/filepath"
	"sort"
	"strings"
)

// https://developer.android.com/build/build-variants
// https://developer.android.com/build/building-cmdline
// https://developer.android.com/tools/adb#am
type gradleAndroidPart struct {
	name, dimension, id, suffix, inherits string
	idSet, suffixSet, signingSet, signed  bool
	identityKnown                         bool
}

type gradleAndroidVariant struct {
	name, id string
	signed   bool
}

func (s *scan) gradleAndroid(path string, nodes []gradleNode, project *gradleProject, application bool, disabled map[string]bool, rule Rule) {
	var android []gradleNode
	identityMutated := false
	signingMutated := false
	for _, node := range nodes {
		if gradleHead(node) == "android" && node.block {
			if android != nil {
				s.issue(path, "scanPartial")
				return
			}
			android = node.body
		} else {
			if gradleContains([]gradleNode{node}, []string{"buildTypes", "productFlavors", "flavorDimensions", "defaultConfig"}) {
				s.issue(path, "scanPartial")
				return
			}
			identityMutated = identityMutated || gradleContains([]gradleNode{node}, []string{"namespace", "applicationId", "applicationIdSuffix", "signingConfig"})
			signingMutated = signingMutated || gradleContains([]gradleNode{node}, []string{"signingConfig"})
		}
		if strings.HasPrefix(gradleHead(node), "androidComponents") || gradleContains([]gradleNode{node}, []string{"variantFilter", "applicationVariants", "libraryVariants", "beforeVariants", "onVariants"}) {
			s.issue(path, "scanPartial")
			return
		}
	}
	if android == nil {
		s.issue(path, "scanPartial")
		return
	}
	variants, namespace, identityKnown, valid := gradleAndroidVariants(android)
	identityKnown = identityKnown && !identityMutated
	if !valid {
		s.issue(path, "scanPartial")
		return
	}
	var activities []string
	if application && identityKnown {
		activities = s.gradleLaunchers(filepath.Dir(path), namespace, android)
	} else if application {
		s.issue(path, "scanPartial")
	}
	for _, variant := range variants {
		if s.ctx.Err() != nil || len(s.result.Candidates) >= maxCommands {
			s.issue(path, "scanLimit")
			return
		}
		variables := map[string]string{"gradleTool": project.tool, "modulePath": project.module, "variant": variant.name}
		if !disabled["assemble"+variant.name] {
			s.addRuleCommand(rule, "assemble", path, project.root, variables, project.platform, true)
		}
		if !application {
			continue
		}
		if !disabled["bundle"+variant.name] {
			s.addRuleCommand(rule, "bundle", path, project.root, variables, project.platform, true)
		}
		if variant.signed && !signingMutated && !disabled["install"+variant.name] {
			s.addRuleCommand(rule, "install", path, project.root, variables, project.platform, true)
		}
		if variant.id == "" {
			s.issue(path, "scanPartial")
			continue
		}
		for _, activity := range activities {
			// Restrict both halves because adb also parses arguments in the
			// remote device shell. Host-shell quoting does not protect `$` there.
			if gradleClass.MatchString(variant.id) && gradleClass.MatchString(activity) {
				s.addRuleCommand(rule, "start", path, project.root, map[string]string{"component": variant.id + "/" + activity}, "", true)
			}
		}
	}
}

func gradleAndroidVariants(android []gradleNode) ([]gradleAndroidVariant, string, bool, bool) {
	base := gradleAndroidPart{identityKnown: true}
	types := map[string]gradleAndroidPart{
		"debug":   {name: "debug", identityKnown: true, signed: true},
		"release": {name: "release", identityKnown: true},
	}
	flavors := []gradleAndroidPart{}
	dimensions := []string{}
	namespace, namespaceKnown := "", true
	for _, node := range android {
		head := gradleHead(node)
		if args, exists := gradleProperty(node, "namespace"); exists {
			namespace, namespaceKnown = gradleLiteral(args)
			continue
		}
		if head == "flavorDimensions" {
			args := node.head[1:]
			if len(args) > 0 && args[0].text == "+" {
				args = args[1:]
			}
			if len(args) > 0 && args[0].text == "=" {
				args = args[1:]
			}
			values, known := gradleStringList(args)
			if !known || len(values) > 4 {
				return nil, "", false, false
			}
			for _, value := range values {
				if !gradleName.MatchString(value) || containsString(dimensions, value) {
					return nil, "", false, false
				}
				dimensions = append(dimensions, value)
			}
			continue
		}
		switch head {
		case "defaultConfig":
			base = gradleAndroidProperties(node.body, base)
		case "buildTypes":
			for _, block := range node.body {
				name, valid := gradleBlockName(block)
				if !valid || !gradleName.MatchString(name) || containsString([]string{"all", "configureEach", "matching", "whenObjectAdded"}, name) || len(types) >= 16 {
					return nil, "", false, false
				}
				part, exists := types[name]
				if !exists {
					part = gradleAndroidPart{name: name, identityKnown: true}
				}
				part = gradleAndroidProperties(block.body, part)
				if part.inherits != "" {
					// Snapshot the source as it exists here. A later debug/release
					// block must not retroactively alter an earlier initWith copy.
					base, known := types[part.inherits]
					if !known {
						return nil, "", false, false
					}
					if !part.suffixSet {
						part.suffix = base.suffix
					}
					if !part.signingSet {
						part.signed = base.signed
					}
					part.identityKnown = part.identityKnown && base.identityKnown
					part.inherits = ""
				}
				types[name] = part
			}
		case "productFlavors":
			for _, block := range node.body {
				name, valid := gradleBlockName(block)
				if !valid || !gradleName.MatchString(name) || containsString([]string{"all", "configureEach", "matching", "whenObjectAdded"}, name) || len(flavors) >= 32 {
					return nil, "", false, false
				}
				for _, property := range block.body {
					if args, exists := gradleProperty(property, "dimension"); exists {
						value, known := gradleLiteral(args)
						if !known || !gradleName.MatchString(value) {
							return nil, "", false, false
						}
					}
				}
				part := gradleAndroidProperties(block.body, gradleAndroidPart{name: name, identityKnown: true})
				flavors = append(flavors, part)
			}
		default:
			if node.block && gradleContains(node.body, []string{"buildTypes", "productFlavors", "flavorDimensions", "applicationId", "applicationIdSuffix"}) {
				return nil, "", false, false
			}
		}
	}
	if len(dimensions) == 0 && len(flavors) > 0 {
		return nil, "", false, false
	}
	groups := map[string][]gradleAndroidPart{}
	seen := map[string]bool{}
	for _, flavor := range flavors {
		if flavor.dimension == "" && len(dimensions) == 1 {
			flavor.dimension = dimensions[0]
		}
		if !containsString(dimensions, flavor.dimension) || seen[flavor.name] {
			return nil, "", false, false
		}
		seen[flavor.name] = true
		groups[flavor.dimension] = append(groups[flavor.dimension], flavor)
	}
	combinations := [][]gradleAndroidPart{{}}
	if len(flavors) > 0 {
		for _, dimension := range dimensions {
			group := groups[dimension]
			if len(group) == 0 || len(combinations)*len(group)*len(types) > 32 {
				return nil, "", false, false
			}
			next := [][]gradleAndroidPart{}
			for _, combination := range combinations {
				for _, flavor := range group {
					copy := append([]gradleAndroidPart(nil), combination...)
					next = append(next, append(copy, flavor))
				}
			}
			combinations = next
		}
	}
	typeNames := make([]string, 0, len(types))
	for name := range types {
		typeNames = append(typeNames, name)
	}
	sort.Strings(typeNames)
	variants := []gradleAndroidVariant{}
	for _, combination := range combinations {
		for _, name := range typeNames {
			buildType := types[name]
			variantName, id, suffix := "", base.id, base.suffix
			identityKnown := base.identityKnown && buildType.identityKnown
			idOverridden := false
			for _, flavor := range combination {
				variantName += gradleCapitalize(flavor.name)
				identityKnown = identityKnown && flavor.identityKnown
				// Earlier flavor dimensions have higher override priority.
				if flavor.idSet && !idOverridden {
					id, idOverridden = flavor.id, true
				}
				suffix += flavor.suffix
			}
			variantName += gradleCapitalize(name)
			if !identityKnown || !gradleClass.MatchString(id+suffix+buildType.suffix) {
				id = ""
			} else {
				id += suffix + buildType.suffix
			}
			variants = append(variants, gradleAndroidVariant{variantName, id, buildType.signed})
		}
	}
	return variants, namespace, namespaceKnown, len(variants) <= 32
}

func gradleAndroidProperties(nodes []gradleNode, part gradleAndroidPart) gradleAndroidPart {
	for _, node := range nodes {
		for _, name := range []string{"applicationId", "applicationIdSuffix", "dimension"} {
			if args, exists := gradleProperty(node, name); exists {
				value, known := gradleLiteral(args)
				part.identityKnown = part.identityKnown && known
				switch name {
				case "applicationId":
					part.id, part.idSet = value, true
				case "applicationIdSuffix":
					part.suffix, part.suffixSet = value, true
					if value != "" && !strings.HasPrefix(value, ".") {
						part.identityKnown = false
					}
				case "dimension":
					part.dimension = value
				}
			}
		}
		if args, exists := gradleProperty(node, "signingConfig"); exists {
			part.signingSet = true
			part.signed = len(args) == 1 && args[0].kind == 'i' && args[0].text == "signingConfigs.debug"
			if len(args) == 4 && args[0].kind == 'i' && args[2].kind == 's' && args[2].text == "debug" {
				part.signed = (args[0].text == "signingConfigs.getByName" && args[1].text == "(" && args[3].text == ")") ||
					(args[0].text == "signingConfigs" && args[1].text == "[" && args[3].text == "]")
			}
		}
		if gradleHead(node) == "initWith" {
			// initWith copies the source at this point; it overwrites properties
			// set earlier in this block. Later literal assignments remain valid.
			part.idSet, part.suffixSet, part.signingSet = false, false, false
			part.id, part.suffix = "", ""
			part.inherits = ""
			args, _ := gradleArguments(node.head[1:])
			if len(args) >= 2 && (args[0].text == "getByName" || args[0].text == "get" || args[0].text == "named") {
				args, _ = gradleArguments(args[1:])
				part.inherits, _ = gradleLiteral(args)
			} else if len(args) == 1 && args[0].kind == 'i' {
				part.inherits = args[0].text
			}
			if !gradleName.MatchString(part.inherits) {
				part.identityKnown, part.signed = false, false
				part.signingSet = true
			}
		}
		if node.block && gradleContains(node.body, []string{"applicationId", "applicationIdSuffix", "signingConfig", "dimension", "initWith"}) {
			part.identityKnown, part.signed = false, false
			part.signingSet = true
		}
	}
	return part
}

func gradleCapitalize(value string) string {
	if value == "" {
		return ""
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func (s *scan) gradleLaunchers(dir, namespace string, android []gradleNode) []string {
	manifest := filepath.Join(dir, "src", "main", "AndroidManifest.xml")
	for _, node := range android {
		if gradleHead(node) == "sourceSets" || gradleContains([]gradleNode{node}, []string{"manifestPlaceholders", "manifest.srcFile", "manifest.srcFiles"}) {
			s.issue(manifest, "scanPartial")
			return nil
		}
	}
	prefix := filepath.Join(dir, "src") + string(filepath.Separator)
	for path := range s.entries {
		if path != manifest && strings.HasPrefix(path, prefix) && filepath.Base(path) == "AndroidManifest.xml" {
			s.issue(manifest, "scanPartial")
			return nil
		}
	}
	raw := s.read(manifest)
	if raw == nil {
		return nil
	}
	launchers, valid := gradleManifestLaunchers(raw, namespace)
	if !valid {
		s.issue(manifest, "scanPartial")
		return nil
	}
	return launchers
}

func gradleManifestLaunchers(data []byte, namespace string) ([]string, bool) {
	const androidNS = "http://schemas.android.com/apk/res/android"
	decoder := xml.NewDecoder(bytes.NewReader(data))
	type activity struct {
		name, target    string
		usable, enabled bool
		launcher, alias bool
	}
	var current *activity
	launchers := []string{}
	declaredActivities := map[string]bool{}
	stack := []string{}
	applicationUsable, inFilter, main, launcher := false, false, false, false
	for count := 0; count < 8192; count++ {
		token, err := decoder.Token()
		if err == io.EOF {
			return launchers, len(stack) == 0
		}
		if err != nil {
			return nil, false
		}
		switch element := token.(type) {
		case xml.StartElement:
			if len(stack) >= 32 {
				return nil, false
			}
			parent := ""
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			stack = append(stack, element.Name.Local)
			attributes := map[string]string{}
			for _, attr := range element.Attr {
				if strings.Contains(attr.Value, "${") || (attr.Name.Space == "http://schemas.android.com/tools" && !containsString([]string{"ignore", "targetApi"}, attr.Name.Local)) {
					return nil, false
				}
				if attr.Name.Space == androidNS {
					attributes[attr.Name.Local] = attr.Value
				}
				if element.Name.Local == "manifest" && attr.Name.Local == "package" && attr.Name.Space == "" && namespace == "" {
					namespace = attr.Value
				}
			}
			switch element.Name.Local {
			case "application":
				applicationUsable = parent == "manifest" && (attributes["enabled"] == "" || attributes["enabled"] == "true")
			case "activity", "activity-alias":
				if parent != "application" || current != nil {
					return nil, false
				}
				name := attributes["name"]
				if strings.HasPrefix(name, ".") {
					name = namespace + name
				} else if !strings.Contains(name, ".") {
					name = namespace + "." + name
				}
				enabled := applicationUsable && (attributes["enabled"] == "" || attributes["enabled"] == "true") && gradleClass.MatchString(name)
				usable := enabled && attributes["exported"] == "true"
				alias := element.Name.Local == "activity-alias"
				target := attributes["targetActivity"]
				if alias {
					if strings.HasPrefix(target, ".") {
						target = namespace + target
					} else if !strings.Contains(target, ".") {
						target = namespace + "." + target
					}
					usable = usable && gradleClass.MatchString(target) && declaredActivities[target]
				}
				current = &activity{name: name, target: target, usable: usable, enabled: enabled, alias: alias}
			case "intent-filter":
				inFilter = current != nil && (parent == "activity" || parent == "activity-alias")
				main, launcher = false, false
			case "action":
				if inFilter && parent == "intent-filter" && attributes["name"] == "android.intent.action.MAIN" {
					main = true
				}
			case "category":
				if inFilter && parent == "intent-filter" && attributes["name"] == "android.intent.category.LAUNCHER" {
					launcher = true
				}
			}
		case xml.EndElement:
			if element.Name.Local == "intent-filter" {
				if inFilter && current != nil && main && launcher {
					current.launcher = true
				}
				inFilter = false
			}
			if element.Name.Local == "activity" || element.Name.Local == "activity-alias" {
				if current != nil && current.launcher && !current.usable {
					return nil, false
				}
				if current != nil && !current.alias {
					declaredActivities[current.name] = current.enabled
				}
				if current != nil && current.usable && current.launcher {
					launchers = appendUnique(launchers, current.name)
					if len(launchers) > 16 {
						return nil, false
					}
				}
				current = nil
			}
			if len(stack) == 0 {
				return nil, false
			}
			stack = stack[:len(stack)-1]
		}
	}
	return nil, false
}
