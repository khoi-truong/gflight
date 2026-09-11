package encoding

import (
	"encoding/json"
	"net/url"
)

// batchKind is the request-kind slot every batchexecute envelope carries. The
// UI sends "generic" for every FlightsFrontendService call.
const batchKind = "generic"

// Batch wraps one method payload in the envelope the batchexecute endpoint
// expects and percent-encodes it for the `f.req` form field.
//
// The envelope is a single-element batch: [[[rpcID, payload, null, "generic"]]],
// where payload is the JSON string an Encode* function returned. The rpc id
// appears here as well as in the query string; both must name the same method.
func Batch(rpcID, payload string) string {
	envelope := []any{[]any{[]any{rpcID, payload, nil, batchKind}}}
	// Marshalling strings and nils cannot fail.
	out, _ := json.Marshal(envelope)
	// Google's UI uses url-encoding that leaves "/" untouched; QueryEscape is
	// a superset of that and the endpoint accepts it.
	return url.QueryEscape(string(out))
}
