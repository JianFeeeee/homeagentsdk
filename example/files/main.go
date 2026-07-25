//go:build !windows || !cgo

package main

import (
	sdk "gitcode.com/JianFeeeee/homeagent-sdk/sdk"
)

func NewPlugin(name string, config map[string]interface{}) (sdk.Plugin, error) {
	return NewPluginFactory(name, config)
}
