# METADATA
# title: HTTP listeners redirect to HTTPS
# description: An HTTP listener, or a listener on port 80, redirects every default action to HTTPS.
# custom:
#   provider: aws
#   resources:
#     - aws_lb_listener
package ergo.templates.aws.lb_http_redirects_to_https

import data.ergo.lib.terraform as tf

deny contains tf.violation(rc, msg) if {
	some rc in tf.resources("aws_lb_listener")
	http_listener(rc)
	not redirects(rc)
	msg := lb_msg(rc)
}

http_listener(rc) if tf.equals(rc, ["protocol"], "HTTP")

http_listener(rc) if {
	tf.equals(rc, ["port"], 80)
	not secure_protocol(rc)
}

http_listener(rc) if {
	tf.unknown(rc, ["protocol"])
	not tf.equals(rc, ["port"], 443)
}

secure_protocol(rc) if tf.equals(rc, ["protocol"], "HTTPS")

secure_protocol(rc) if tf.equals(rc, ["protocol"], "TLS")

secure_protocol(rc) if tf.equals(rc, ["protocol"], "TCP")

secure_protocol(rc) if tf.equals(rc, ["protocol"], "UDP")

secure_protocol(rc) if tf.equals(rc, ["protocol"], "TCP_UDP")

secure_protocol(rc) if tf.equals(rc, ["protocol"], "GENEVE")

redirects(rc) if {
	actions := object.get(rc.change.after, "default_action", [])
	count(actions) > 0
	every i, action in actions {
		is_object(action)
		tf.equals(rc, ["default_action", i, "type"], "redirect")
		some j, redir in object.get(action, "redirect", [])
		is_object(redir)
		tf.equals(rc, ["default_action", i, "redirect", j, "protocol"], "HTTPS")
	}
}

lb_msg(rc) := sprintf("%s protocol is unknown at plan time", [rc.address]) if {
	tf.unknown(rc, ["protocol"])
} else := sprintf("%s default_action is unknown at plan time", [rc.address]) if {
	tf.unknown(rc, ["default_action"])
} else := sprintf("%s HTTP listener must redirect to HTTPS", [rc.address])
