package demo

import rego.v1

deny contains msg if {
	msg := missing
}
