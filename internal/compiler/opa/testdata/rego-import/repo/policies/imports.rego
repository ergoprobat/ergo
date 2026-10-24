package demo

import rego.v1
import data.other.pkg

deny contains msg if {
	msg := "no"
}
