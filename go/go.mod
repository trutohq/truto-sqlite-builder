module github.com/trutohq/truto-sqlite-builder/go

go 1.27.0

require (
	github.com/trutohq/differential v0.0.0
	github.com/trutohq/jsorder v0.0.0
)

require (
	github.com/recolabs/gnata v0.4.0 // indirect
	github.com/tidwall/gjson v1.18.0 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
)

// jsorder is unpublished — branch feat/jsorder-initial in ~/work/jsorder, not
// pushed anywhere yet. When it is, delete this directive and pin a real
// version; nothing else about the dependency changes.
replace github.com/trutohq/jsorder => ../../jsorder

replace github.com/trutohq/differential => ../../differential
