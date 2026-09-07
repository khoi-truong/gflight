// Package encoding will hold the wire encoding for Google Flights requests —
// the base64-wrapped protobuf `tfs` parameter and the batchexecute response
// framing around it.
//
// It is deliberately internal: the upstream format is undocumented and will
// churn, and nothing here should ever appear in the module's public API or
// force a breaking release.
package encoding
