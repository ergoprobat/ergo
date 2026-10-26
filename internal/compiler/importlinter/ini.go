package importlinter

import (
	"bytes"
	"regexp"
	"strings"
	"unicode/utf8"
)

// iniValue is one config value. A continuation makes it a list, which is how
// import-linter splits a multiline value. A single line stays a scalar, and a
// list field accepts that scalar as its one item.
type iniValue struct {
	list   bool
	scalar string
	items  []string
}

type iniContract struct {
	id     string
	values map[string]iniValue
}

type iniFile struct {
	session   map[string]iniValue
	contracts []iniContract
}

var contractSection = regexp.MustCompile(`^\[importlinter:contract:([a-z][a-z0-9]*(?:-[a-z0-9]+)*)\]$`)

var keyName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// parseINI reads the subset of configparser syntax import-linter 2.15 accepts
// from an .importlinter file. Percent signs are rejected because configparser
// treats them as interpolation. A dotted contract id is rejected because
// import-linter keeps only the text after the last colon.
func parseINI(src []byte) (iniFile, error) {
	if bytes.HasPrefix(src, []byte{0xEF, 0xBB, 0xBF}) {
		return iniFile{}, fail(CodeINI, "byte order mark")
	}
	if !utf8.Valid(src) {
		return iniFile{}, fail(CodeINI, "not utf-8")
	}
	if bytes.Contains(src, []byte{'%'}) {
		return iniFile{}, fail(CodeINI, "percent signs are interpolation")
	}
	for _, b := range src {
		if b == '\n' || b == '\t' || b >= 0x20 {
			continue
		}
		return iniFile{}, fail(CodeINI, "control character")
	}

	var file iniFile
	var (
		kind     string
		id       string
		current  map[string]iniValue
		session  bool
		seenID   = map[string]struct{}{}
		building bool
		key      string
		first    string
		extra    []string
	)
	flushKey := func() {
		if !building {
			return
		}
		current[key] = iniValueFrom(first, extra)
		building = false
		key = ""
		first = ""
		extra = nil
	}
	flushSection := func() {
		flushKey()
		if kind == "" {
			return
		}
		if kind == "session" {
			file.session = current
		} else {
			file.contracts = append(file.contracts, iniContract{id: id, values: current})
		}
		kind = ""
		current = nil
	}

	for _, line := range strings.Split(string(src), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			continue
		}
		if line[0] == ' ' || line[0] == '\t' {
			if !building {
				return iniFile{}, fail(CodeINI, "continuation without a key")
			}
			extra = append(extra, trimmed)
			continue
		}
		if strings.HasPrefix(trimmed, "[") {
			flushSection()
			nextKind, nextID, err := parseSection(trimmed)
			if err != nil {
				return iniFile{}, err
			}
			if nextKind == "session" {
				if session {
					return iniFile{}, fail(CodeINI, "duplicate section importlinter")
				}
				session = true
			} else {
				if _, ok := seenID[nextID]; ok {
					return iniFile{}, fail(CodeINI, "duplicate contract %q", nextID)
				}
				seenID[nextID] = struct{}{}
			}
			kind = nextKind
			id = nextID
			current = map[string]iniValue{}
			continue
		}
		if kind == "" {
			return iniFile{}, fail(CodeINI, "key outside a section")
		}
		flushKey()
		parsedKey, parsedValue, err := parseKey(trimmed)
		if err != nil {
			return iniFile{}, err
		}
		if _, ok := current[parsedKey]; ok {
			return iniFile{}, fail(CodeINI, "duplicate key %q", parsedKey)
		}
		key = parsedKey
		first = parsedValue
		building = true
	}
	flushSection()
	if !session {
		return iniFile{}, fail(CodeINI, "missing importlinter section")
	}
	return file, nil
}

func parseSection(line string) (kind, id string, err error) {
	if line == "[importlinter]" {
		return "session", "", nil
	}
	match := contractSection.FindStringSubmatch(line)
	if match == nil {
		return "", "", fail(CodeINI, "section %s", line)
	}
	return "contract", match[1], nil
}

func parseKey(line string) (string, string, error) {
	eq := strings.IndexByte(line, '=')
	colon := strings.IndexByte(line, ':')
	idx := eq
	if idx < 0 || (colon >= 0 && colon < idx) {
		idx = colon
	}
	if idx <= 0 {
		return "", "", fail(CodeINI, "expected key = value")
	}
	key := strings.TrimSpace(line[:idx])
	if !keyName.MatchString(key) {
		return "", "", fail(CodeINI, "key %q", key)
	}
	return key, strings.TrimSpace(line[idx+1:]), nil
}

func iniValueFrom(first string, extra []string) iniValue {
	if len(extra) == 0 {
		return iniValue{scalar: first}
	}
	var items []string
	if first != "" {
		items = append(items, first)
	}
	items = append(items, extra...)
	return iniValue{list: true, items: items}
}
