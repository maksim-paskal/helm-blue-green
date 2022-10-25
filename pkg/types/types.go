package types

import "fmt"

type Version struct {
	Scope string
	Value string
}

func NewVersion(scope, value string) Version {
	return Version{
		Scope: scope,
		Value: value,
	}
}

func (v Version) String() string {
	return fmt.Sprintf("%s=%s", v.Key(), v.Value)
}

func (v Version) Key() string {
	return fmt.Sprintf("%s/version", v.Scope)
}
