package web

import "strings"

func sanitizeText(value string) string {
	if value == "" {
		return ""
	}

	// Ensure valid UTF-8 and strip control chars that can break parsing or JS in templates.
	value = strings.ToValidUTF8(value, "")
	value = strings.Map(func(r rune) rune {
		if r < 32 {
			return -1
		}
		return r
	}, value)

	value = strings.ReplaceAll(value, "\\", " ")
	value = strings.ReplaceAll(value, "\"", " ")
	value = strings.ReplaceAll(value, "'", " ")
	value = strings.ReplaceAll(value, "\t", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.TrimSpace(value)

	for strings.Contains(value, "  ") {
		value = strings.ReplaceAll(value, "  ", " ")
	}

	return value
}

func sanitizeConfig(value string) string {
	if value == "" {
		return ""
	}
	// Keep original payload as much as possible, only ensure valid UTF-8 and remove control chars.
	value = strings.ToValidUTF8(value, "")
	value = strings.Map(func(r rune) rune {
		if r < 32 {
			return -1
		}
		return r
	}, value)
	return strings.TrimSpace(value)
}
