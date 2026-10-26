package importlinter

import "strings"

func encode(sess session, contracts []outContract) []byte {
	var b strings.Builder
	b.WriteString("[importlinter]\n")
	writeKeys(&b, sessionValues(sess))
	for _, contract := range contracts {
		b.WriteString("\n[importlinter:contract:")
		b.WriteString(contract.id)
		b.WriteString("]\n")
		writeKeys(&b, contractValues(contract))
	}
	return []byte(b.String())
}

func sessionValues(sess session) map[string]iniValue {
	values := map[string]iniValue{
		"root_packages": {list: true, items: sess.roots},
	}
	if sess.include != nil {
		values["include_external_packages"] = boolText(*sess.include)
	}
	if sess.exclude != nil {
		values["exclude_type_checking_imports"] = boolText(*sess.exclude)
	}
	return values
}

func contractValues(contract outContract) map[string]iniValue {
	values := map[string]iniValue{
		"name": {scalar: contract.name},
		"type": {scalar: contract.typeName},
	}
	for key, val := range contract.fields {
		values[key] = val
	}
	return values
}

func boolText(val bool) iniValue {
	if val {
		return iniValue{scalar: "True"}
	}
	return iniValue{scalar: "False"}
}

func writeKeys(b *strings.Builder, values map[string]iniValue) {
	for _, key := range sortedKeys(values) {
		val := values[key]
		if val.list {
			b.WriteString(key)
			b.WriteString(" =\n")
			for _, item := range val.items {
				b.WriteString("    ")
				b.WriteString(item)
				b.WriteString("\n")
			}
			continue
		}
		b.WriteString(key)
		b.WriteString(" = ")
		b.WriteString(val.scalar)
		b.WriteString("\n")
	}
}
