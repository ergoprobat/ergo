package importlinter

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var ident = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type fieldKind int

const (
	kindModules fieldKind = iota
	kindImports
	kindBool
	kindInt
	kindLayers
	kindNames
)

type fieldSpec struct {
	kind     fieldKind
	required bool
}

// sessionKeys import-linter would read. contract_types is rejected because
// import-linter imports the classes it names. show_timings only shapes a report.
var sessionRejected = map[string]struct{}{
	"contract_types": {},
	"show_timings":   {},
}

// contractRejected are fields import-linter accepts and this compiler does not.
// unmatched_ignore_imports_alerting stays at its default, error, so a stale
// ignore_imports entry still breaks the contract. broken_contract_guidance is report text.
var contractRejected = map[string]struct{}{
	"broken_contract_guidance":          {},
	"id":                                {},
	"unmatched_ignore_imports_alerting": {},
}

var contractFields = map[string]map[string]fieldSpec{
	"forbidden": {
		"allow_indirect_imports": {kind: kindBool},
		"as_packages":            {kind: kindBool},
		"forbidden_modules":      {kind: kindModules, required: true},
		"ignore_imports":         {kind: kindImports},
		"source_modules":         {kind: kindModules, required: true},
	},
	"layers": {
		"containers":         {kind: kindModules},
		"exhaustive":         {kind: kindBool},
		"exhaustive_ignores": {kind: kindNames},
		"ignore_imports":     {kind: kindImports},
		"layers":             {kind: kindLayers, required: true},
	},
	"independence": {
		"ignore_imports": {kind: kindImports},
		"modules":        {kind: kindModules, required: true},
	},
	"protected": {
		"allowed_importers": {kind: kindModules, required: true},
		"as_packages":       {kind: kindBool},
		"ignore_imports":    {kind: kindImports},
		"protected_modules": {kind: kindModules, required: true},
	},
	"acyclic_siblings": {
		"ancestors":        {kind: kindModules, required: true},
		"depth":            {kind: kindInt},
		"ignore_imports":   {kind: kindImports},
		"skip_descendants": {kind: kindModules},
	},
}

type session struct {
	roots   []string
	include *bool
	exclude *bool
}

type outContract struct {
	id       string
	name     string
	typeName string
	fields   map[string]iniValue
}

func compileConfig(src []byte) ([]byte, error) {
	file, err := parseINI(src)
	if err != nil {
		return nil, err
	}
	sess, err := validateSession(file.session)
	if err != nil {
		return nil, err
	}
	if len(file.contracts) == 0 {
		return nil, fail(CodeContract, "no contracts")
	}
	contracts := make([]outContract, 0, len(file.contracts))
	for _, contract := range file.contracts {
		out, err := validateContract(contract)
		if err != nil {
			return nil, err
		}
		contracts = append(contracts, out)
	}
	return encode(sess, contracts), nil
}

func validateSession(values map[string]iniValue) (session, error) {
	var sess session
	var sawPackage, sawPackages bool
	for _, key := range sortedKeys(values) {
		val := values[key]
		switch key {
		case "root_package":
			sawPackage = true
			if val.list {
				return session{}, fail(CodeRoot, "root_package is one name")
			}
			if err := checkRoot(val.scalar); err != nil {
				return session{}, err
			}
			sess.roots = []string{val.scalar}
		case "root_packages":
			sawPackages = true
			items, err := listItems(val)
			if err != nil {
				return session{}, fail(CodeRoot, "root_packages is empty")
			}
			for _, item := range items {
				if err := checkRoot(item); err != nil {
					return session{}, err
				}
			}
			if err := duplicates(items); err != nil {
				return session{}, fail(CodeRoot, "duplicate root package")
			}
			sess.roots = items
		case "include_external_packages":
			parsed, err := parseBool(val)
			if err != nil {
				return session{}, err
			}
			sess.include = &parsed
		case "exclude_type_checking_imports":
			parsed, err := parseBool(val)
			if err != nil {
				return session{}, err
			}
			sess.exclude = &parsed
		default:
			if _, rejected := sessionRejected[key]; rejected {
				return session{}, fail(CodeKey, "rejected key %q", key)
			}
			return session{}, fail(CodeKey, "unknown key %q", key)
		}
	}
	if sawPackage && sawPackages {
		return session{}, fail(CodeRoot, "root_package and root_packages")
	}
	if len(sess.roots) == 0 {
		return session{}, fail(CodeRoot, "missing root package")
	}
	return sess, nil
}

func validateContract(contract iniContract) (outContract, error) {
	typeVal, ok := contract.values["type"]
	if !ok || typeVal.list || typeVal.scalar == "" {
		return outContract{}, fail(CodeContract, "contract %s has no type", contract.id)
	}
	spec, ok := contractFields[typeVal.scalar]
	if !ok {
		return outContract{}, fail(CodeContract, "type %q", typeVal.scalar)
	}
	nameVal, ok := contract.values["name"]
	if !ok || nameVal.list || nameVal.scalar == "" {
		return outContract{}, fail(CodeContract, "contract %s has no name", contract.id)
	}
	if !validName(nameVal.scalar) {
		return outContract{}, fail(CodeValue, "name %q", nameVal.scalar)
	}
	fields := map[string]iniValue{}
	for _, key := range sortedKeys(contract.values) {
		if key == "name" || key == "type" {
			continue
		}
		if _, rejected := contractRejected[key]; rejected {
			return outContract{}, fail(CodeKey, "rejected key %q", key)
		}
		field, known := spec[key]
		if !known {
			return outContract{}, fail(CodeKey, "unknown key %q", key)
		}
		cleaned, err := checkField(field, contract.values[key])
		if err != nil {
			return outContract{}, err
		}
		fields[key] = cleaned
	}
	for _, key := range sortedSpecKeys(spec) {
		if spec[key].required {
			if _, ok := fields[key]; !ok {
				return outContract{}, fail(CodeContract, "contract %s misses %s", contract.id, key)
			}
		}
	}
	if typeVal.scalar == "layers" {
		if layers, ok := fields["layers"]; !ok || len(layers.items) < 2 {
			return outContract{}, fail(CodeValue, "layers needs at least two layers")
		}
		if exh, ok := fields["exhaustive"]; ok && exh.scalar == "True" {
			if _, ok := fields["containers"]; !ok {
				return outContract{}, fail(CodeValue, "exhaustive requires containers")
			}
		}
	}
	if typeVal.scalar == "independence" {
		if modules, ok := fields["modules"]; !ok || len(modules.items) < 2 {
			return outContract{}, fail(CodeValue, "independence needs at least two modules")
		}
	}
	return outContract{
		id:       contract.id,
		name:     nameVal.scalar,
		typeName: typeVal.scalar,
		fields:   fields,
	}, nil
}

func checkField(field fieldSpec, val iniValue) (iniValue, error) {
	switch field.kind {
	case kindBool:
		parsed, err := parseBool(val)
		if err != nil {
			return iniValue{}, err
		}
		if parsed {
			return iniValue{scalar: "True"}, nil
		}
		return iniValue{scalar: "False"}, nil
	case kindInt:
		return checkInt(val)
	case kindLayers:
		return checkLayers(val)
	case kindImports:
		return checkList(val, normalizeImport)
	case kindNames:
		return checkList(val, func(item string) (string, error) {
			if !plainModule(item) {
				return "", fail(CodeValue, "module %q", item)
			}
			return item, nil
		})
	default:
		return checkList(val, func(item string) (string, error) {
			if !moduleExpr(item) {
				return "", fail(CodeValue, "module %q", item)
			}
			return item, nil
		})
	}
}

func checkInt(val iniValue) (iniValue, error) {
	if val.list {
		return iniValue{}, fail(CodeValue, "expected one integer")
	}
	number, err := strconv.Atoi(val.scalar)
	if err != nil || number < 0 || strconv.Itoa(number) != val.scalar {
		return iniValue{}, fail(CodeValue, "integer %q", val.scalar)
	}
	return iniValue{scalar: val.scalar}, nil
}

func checkLayers(val iniValue) (iniValue, error) {
	return checkList(val, normalizeLayer)
}

func checkList(val iniValue, clean func(string) (string, error)) (iniValue, error) {
	items, err := listItems(val)
	if err != nil {
		return iniValue{}, err
	}
	out := make([]string, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		norm, err := clean(item)
		if err != nil {
			return iniValue{}, err
		}
		if _, ok := seen[norm]; ok {
			return iniValue{}, fail(CodeValue, "duplicate %q", norm)
		}
		seen[norm] = struct{}{}
		out = append(out, norm)
	}
	return iniValue{list: true, items: out}, nil
}

func listItems(val iniValue) ([]string, error) {
	if !val.list {
		if val.scalar == "" {
			return nil, fail(CodeValue, "empty")
		}
		return []string{val.scalar}, nil
	}
	if len(val.items) == 0 {
		return nil, fail(CodeValue, "empty")
	}
	return val.items, nil
}

func parseBool(val iniValue) (bool, error) {
	if val.list {
		return false, fail(CodeValue, "expected one boolean")
	}
	switch strings.ToLower(val.scalar) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fail(CodeValue, "boolean %q", val.scalar)
	}
}

func checkRoot(name string) error {
	if strings.Contains(name, ".") {
		return fail(CodeRoot, "dotted root package %q", name)
	}
	if !ident.MatchString(name) {
		return fail(CodeRoot, "root package %q", name)
	}
	return nil
}

func duplicates(items []string) error {
	seen := map[string]struct{}{}
	for _, item := range items {
		if _, ok := seen[item]; ok {
			return fail(CodeValue, "duplicate %q", item)
		}
		seen[item] = struct{}{}
	}
	return nil
}

func normalizeLayer(line string) (string, error) {
	if strings.Contains(line, "|") && strings.Contains(line, ":") {
		return "", fail(CodeValue, "layer %q mixes separators", line)
	}
	delim := ""
	parts := []string{line}
	switch {
	case strings.Contains(line, "|"):
		delim = " | "
		parts = strings.Split(line, "|")
	case strings.Contains(line, ":"):
		delim = " : "
		parts = strings.Split(line, ":")
	}
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return "", fail(CodeValue, "empty layer")
		}
		optional := false
		if strings.HasPrefix(part, "(") && strings.HasSuffix(part, ")") && len(part) > 2 {
			optional = true
			part = strings.TrimSpace(part[1 : len(part)-1])
		}
		if !moduleExpr(part) {
			return "", fail(CodeValue, "layer %q", part)
		}
		if _, ok := seen[part]; ok {
			return "", fail(CodeValue, "duplicate layer %q", part)
		}
		seen[part] = struct{}{}
		if optional {
			part = "(" + part + ")"
		}
		out = append(out, part)
	}
	return strings.Join(out, delim), nil
}

func normalizeImport(line string) (string, error) {
	left, right, ok := strings.Cut(line, "->")
	if !ok || strings.Contains(right, "->") {
		return "", fail(CodeValue, "import %q", line)
	}
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if !moduleExpr(left) || !moduleExpr(right) {
		return "", fail(CodeValue, "import %q", line)
	}
	return left + " -> " + right, nil
}

func moduleExpr(expr string) bool {
	if expr == "" || strings.HasPrefix(expr, ".") || strings.HasSuffix(expr, ".") {
		return false
	}
	lastWild := ""
	for _, part := range strings.Split(expr, ".") {
		if part == "*" || part == "**" {
			if lastWild == "**" || (lastWild == "*" && part == "**") {
				return false
			}
			lastWild = part
			continue
		}
		if strings.Contains(part, "*") || !ident.MatchString(part) {
			return false
		}
		lastWild = ""
	}
	return true
}

func plainModule(name string) bool {
	if name == "" {
		return false
	}
	for _, part := range strings.Split(name, ".") {
		if !ident.MatchString(part) {
			return false
		}
	}
	return true
}

func validName(name string) bool {
	if name == "" || len(name) > 200 || strings.TrimSpace(name) != name {
		return false
	}
	if strings.ContainsAny(name, "[]") {
		return false
	}
	for _, r := range name {
		if r < 0x20 || r > 0x7e {
			return false
		}
	}
	return true
}

func sortedKeys(values map[string]iniValue) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedSpecKeys(spec map[string]fieldSpec) []string {
	keys := make([]string, 0, len(spec))
	for key := range spec {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
