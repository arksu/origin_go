package objectdefs

// ResolveAppearanceResource resolves visual resource by flags and appearance rules.
// Rules are evaluated in definition order; first matching rule wins.
func ResolveAppearanceResource(def *ObjectDef, flags []string) string {
	if def == nil {
		return ""
	}
	if len(def.Appearance) == 0 {
		return def.Resource
	}

	flagSet := make(map[string]struct{}, len(flags))
	for _, f := range flags {
		flagSet[f] = struct{}{}
	}

	for _, appearance := range def.Appearance {
		if appearance.When == nil || len(appearance.When.Flags) == 0 {
			if appearance.Resource != "" {
				return appearance.Resource
			}
			return def.Resource
		}

		matched := true
		for _, needed := range appearance.When.Flags {
			if _, ok := flagSet[needed]; !ok {
				matched = false
				break
			}
		}
		if matched {
			if appearance.Resource != "" {
				return appearance.Resource
			}
			return def.Resource
		}
	}

	return def.Resource
}
