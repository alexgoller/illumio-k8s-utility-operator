package controller

// autoCreatableKeys are the standard Illumio label dimensions the operator may
// mint in unknown-label "create" mode. Any other key is rejected so a typo'd key
// cannot spawn a junk label dimension.
var autoCreatableKeys = map[string]bool{"role": true, "app": true, "env": true, "loc": true}

func autoCreatableKey(key string) bool { return autoCreatableKeys[key] }
