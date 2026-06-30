package controller

import microv1 "github.com/alexgoller/illumio-k8s-utility-operator/api/v1alpha1"

// autoCreatableKeys are the standard Illumio label dimensions the operator may
// mint in unknown-label "create" mode. Any other key is rejected so a typo'd key
// cannot spawn a junk label dimension.
var autoCreatableKeys = map[string]bool{
	microv1.LabelKeyRole: true,
	microv1.LabelKeyApp:  true,
	microv1.LabelKeyEnv:  true,
	microv1.LabelKeyLoc:  true,
}

func autoCreatableKey(key string) bool { return autoCreatableKeys[key] }
