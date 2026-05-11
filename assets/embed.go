package assets

import _ "embed"

// NATSDocsMasterZip is the embedded nats.docs archive used by the standalone CLI.
//
//go:embed nats.docs-master.zip
var NATSDocsMasterZip []byte
