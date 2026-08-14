package store

type ValueKind uint8

const (
	ValueKindUnknown ValueKind = iota
	ValueKindText
	ValueKindJSON
	ValueKindBinary
)
