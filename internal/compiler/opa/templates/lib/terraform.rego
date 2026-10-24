# Shared helpers for built-in Terraform plan templates.
# Repository policies cannot import this module.
# Schemas: hashicorp/aws 6.x and hashicorp/azurerm 5.x.
package ergo.lib.terraform

# resources keeps managed changes that still have an after state.
# Delete-only changes have a null after state and are omitted.
# A value in after_unknown counts as set for is_set and fails equals.
resources(type) := [rc |
	some rc in object.get(input, "resource_changes", [])
	rc.mode == "managed"
	rc.type == type
	rc.change.after != null
]

# unknown is true when after_unknown is true at path or at a prefix of path.
# walk follows the whole value, at any depth.
unknown(rc, path) if {
	walk(object.get(rc.change, "after_unknown", {}), [walked, value])
	value == true
	path_has_prefix(path, walked)
}

path_has_prefix(path, prefix) if {
	is_array(path)
	is_array(prefix)
	count(prefix) <= count(path)
	every i, item in prefix {
		path[i] == item
	}
}

is_set(rc, path) if unknown(rc, path)

is_set(rc, path) if {
	not unknown(rc, path)
	value_at(rc.change.after, path) != null
}

equals(rc, path, want) if {
	not unknown(rc, path)
	value_at(rc.change.after, path) == want
}

value_at(node, path) := found if {
	walk(node, [walked, found])
	count(walked) == count(path)
	every i, item in walked {
		path[i] == item
	}
}

violation(rc, msg) := {"address": rc.address, "msg": msg}

# require_msg is defined when path is missing, different from want, or unknown.
require_msg(rc, path, want, attr) := sprintf("%s %s is unknown at plan time", [rc.address, attr]) if {
	unknown(rc, path)
	want == want
}

require_msg(rc, path, want, attr) := sprintf("%s %s must be %s", [rc.address, attr, want]) if {
	not unknown(rc, path)
	not equals(rc, path, want)
	is_string(want)
}

require_msg(rc, path, want, attr) := sprintf("%s %s must be %v", [rc.address, attr, want]) if {
	not unknown(rc, path)
	not equals(rc, path, want)
	not is_string(want)
}

# forbid_msg is defined when path is unknown or equal to bad.
# A missing value is allowed.
forbid_msg(rc, path, bad, attr) := sprintf("%s %s is unknown at plan time", [rc.address, attr]) if {
	unknown(rc, path)
	bad == bad
}

forbid_msg(rc, path, bad, attr) := sprintf("%s %s must not be true", [rc.address, attr]) if {
	not unknown(rc, path)
	equals(rc, path, bad)
	bad == true
}

forbid_msg(rc, path, bad, attr) := sprintf("%s %s must not be %s", [rc.address, attr, bad]) if {
	not unknown(rc, path)
	equals(rc, path, bad)
	is_string(bad)
}

# same_bucket pairs S3 resources in one module.
# A known bucket attribute matches the bucket name or id.
# An unknown bucket attribute matches a configuration reference.
same_bucket(child, owner) if {
	same_module(child.address, owner.address)
	name := value_at(child.change.after, ["bucket"])
	is_string(name)
	name != ""
	bucket_named(owner, name)
}

same_bucket(child, owner) if {
	same_module(child.address, owner.address)
	unknown(child, ["bucket"])
	some ref in attr_refs(child.address, "bucket")
	refers(ref, owner.address)
}

bucket_named(owner, name) if value_at(owner.change.after, ["bucket"]) == name

bucket_named(owner, name) if value_at(owner.change.after, ["id"]) == name

refers(ref, address) if ref == address

refers(ref, address) if startswith(ref, sprintf("%s.", [address]))

same_module(left, right) if module_prefix(left) == module_prefix(right)

module_prefix(address) := "" if module_end(split(address, ".")) == 0

module_prefix(address) := concat(".", array.slice(split(address, "."), 0, module_end(split(address, ".")))) if {
	module_end(split(address, ".")) > 0
}

# module_end is twice the number of leading module.name pairs.
module_end(parts) := 0 if count(parts) == 0

module_end(parts) := count(hits) * 2 if {
	count(parts) > 0
	hits := [i |
		some i in numbers.range(0, count(parts) - 1)
		i % 2 == 0
		i + 1 < count(parts)
		parts[i] == "module"
		module_chain(parts, i)
	]
}

module_chain(parts, i) if {
	every j in numbers.range(0, i) {
		even_is_module(parts, j)
	}
}

even_is_module(parts, j) if {
	j % 2 == 1
	count(parts) >= 0
}

even_is_module(parts, j) if {
	j % 2 == 0
	parts[j] == "module"
}

attr_refs(address, attr) := [ref |
	some resource in config_resources
	resource.address == address
	some ref in object.get(object.get(object.get(resource, "expressions", {}), attr, {}), "references", [])
]

config_resources contains resource if {
	walk(object.get(input, "configuration", {}), [_, value])
	is_object(value)
	is_string(object.get(value, "address", ""))
	object.get(value, "address", "") != ""
	is_object(object.get(value, "expressions", null))
	resource := value
}

world_cidr(cidr) if cidr == "0.0.0.0/0"

world_cidr(cidr) if cidr == "::/0"

world_source(value) if world_cidr(value)

world_source(value) if value == "*"

world_source(value) if {
	is_string(value)
	lower(value) == "internet"
}

includes_port(from, to, port) if {
	is_number(from)
	is_number(to)
	from <= port
	to >= port
}

tcp_or_all(proto) if proto == "-1"

tcp_or_all(proto) if proto == "*"

tcp_or_all(proto) if proto == "all"

tcp_or_all(proto) if {
	is_string(proto)
	lower(proto) == "tcp"
}

aws_exposed(rc, path, port) if {
	rule := value_at(rc.change.after, path)
	aws_l4(rule)
	aws_world(rule)
	aws_covers(rule, port)
}

aws_exposed(rc, path, port) if {
	rule := value_at(rc.change.after, path)
	aws_l4(rule)
	aws_world(rule)
	unknown(rc, array.concat(path, ["from_port"]))
	is_number(port)
}

aws_exposed(rc, path, port) if {
	rule := value_at(rc.change.after, path)
	aws_l4(rule)
	aws_world(rule)
	unknown(rc, array.concat(path, ["to_port"]))
	is_number(port)
}

aws_exposed(rc, path, port) if {
	rule := value_at(rc.change.after, path)
	aws_l4(rule)
	aws_covers(rule, port)
	aws_cidr_unknown(rc, path)
}

aws_l4(rule) if tcp_or_all(object.get(rule, "protocol", null))

aws_l4(rule) if tcp_or_all(object.get(rule, "ip_protocol", null))

aws_world(rule) if {
	some cidr in object.get(rule, "cidr_blocks", [])
	world_cidr(cidr)
}

aws_world(rule) if {
	some cidr in object.get(rule, "ipv6_cidr_blocks", [])
	world_cidr(cidr)
}

aws_world(rule) if world_cidr(object.get(rule, "cidr_ipv4", ""))

aws_world(rule) if world_cidr(object.get(rule, "cidr_ipv6", ""))

aws_covers(rule, port) if includes_port(object.get(rule, "from_port", null), object.get(rule, "to_port", null), port)

aws_covers(rule, port) if {
	is_number(port)
	object.get(rule, "protocol", object.get(rule, "ip_protocol", "")) == "-1"
	object.get(rule, "from_port", null) == 0
	object.get(rule, "to_port", null) == 0
}

aws_cidr_unknown(rc, path) if unknown(rc, array.concat(path, ["cidr_blocks"]))

aws_cidr_unknown(rc, path) if unknown(rc, array.concat(path, ["ipv6_cidr_blocks"]))

aws_cidr_unknown(rc, path) if unknown(rc, array.concat(path, ["cidr_ipv4"]))

aws_cidr_unknown(rc, path) if unknown(rc, array.concat(path, ["cidr_ipv6"]))

aws_expose_msg(rc, path, label) := sprintf("%s %s from_port is unknown at plan time", [rc.address, label]) if {
	unknown(rc, array.concat(path, ["from_port"]))
} else := sprintf("%s %s to_port is unknown at plan time", [rc.address, label]) if {
	unknown(rc, array.concat(path, ["to_port"]))
} else := sprintf("%s %s cidr is unknown at plan time", [rc.address, label]) if {
	aws_cidr_unknown(rc, path)
} else := sprintf("%s allows %s from the internet", [rc.address, label])

azure_exposed(rc, path, port) if {
	rule := value_at(rc.change.after, path)
	azure_inbound_allow(rule)
	azure_tcp(rule)
	azure_world(rule)
	azure_port(rule, port)
}

azure_exposed(rc, path, port) if {
	rule := value_at(rc.change.after, path)
	azure_inbound_allow(rule)
	azure_tcp(rule)
	azure_source_unknown(rc, path)
	azure_port(rule, port)
}

azure_exposed(rc, path, port) if {
	rule := value_at(rc.change.after, path)
	azure_inbound_allow(rule)
	azure_tcp(rule)
	azure_world(rule)
	azure_port_unknown(rc, path)
	is_number(port)
}

azure_inbound_allow(rule) if {
	direction := object.get(rule, "direction", "")
	access := object.get(rule, "access", "")
	is_string(direction)
	is_string(access)
	lower(direction) == "inbound"
	lower(access) == "allow"
}

azure_tcp(rule) if {
	is_string(object.get(rule, "protocol", ""))
	lower(object.get(rule, "protocol", "")) == "tcp"
}

azure_tcp(rule) if object.get(rule, "protocol", "") == "*"

azure_world(rule) if world_source(object.get(rule, "source_address_prefix", ""))

azure_world(rule) if {
	some value in object.get(rule, "source_address_prefixes", [])
	world_source(value)
}

azure_port(rule, port) if azure_port_value(object.get(rule, "destination_port_range", ""), port)

azure_port(rule, port) if {
	some value in object.get(rule, "destination_port_ranges", [])
	azure_port_value(value, port)
}

azure_port_value(value, port) if {
	value == "*"
	is_number(port)
}

azure_port_value(value, port) if value == sprintf("%d", [port])

azure_port_value(value, port) if {
	parts := split(value, "-")
	count(parts) == 2
	to_number(parts[0]) <= port
	to_number(parts[1]) >= port
}

azure_source_unknown(rc, path) if unknown(rc, array.concat(path, ["source_address_prefix"]))

azure_source_unknown(rc, path) if unknown(rc, array.concat(path, ["source_address_prefixes"]))

azure_port_unknown(rc, path) if unknown(rc, array.concat(path, ["destination_port_range"]))

azure_port_unknown(rc, path) if unknown(rc, array.concat(path, ["destination_port_ranges"]))

azure_expose_msg(rc, path, label) := sprintf("%s %s destination_port_range is unknown at plan time", [rc.address, label]) if {
	azure_port_unknown(rc, path)
} else := sprintf("%s %s source_address_prefix is unknown at plan time", [rc.address, label]) if {
	azure_source_unknown(rc, path)
} else := sprintf("%s allows %s from the internet", [rc.address, label])
