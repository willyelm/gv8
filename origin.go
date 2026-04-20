package gv8

// ScriptOrigin mirrors the key parts of V8's ScriptOrigin that matter for
// embedding and diagnostics.
type ScriptOrigin struct {
	ResourceName string
	LineOffset   int
	ColumnOffset int
	IsModule     bool
}
