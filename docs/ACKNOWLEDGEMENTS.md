# Acknowledgements

`gflight` talks to undocumented Google Flights endpoints. None of that protocol
knowledge was discovered here — it is the accumulated reverse-engineering of
several prior projects, all MIT-licensed. We ported the _design and the
protocol facts_, not source code, so no third-party copyright headers appear in
the `.go` files. The credit still belongs upstream:

| Project | What we learned from it |
| --- | --- |
| [punitarani/fli](https://github.com/punitarani/fli) (MIT) | The overall architecture: the `FlightsFrontendService` RPC approach, the `f.req` index map, the batchexecute response framing, the row-decoder layout, hand-rolling protobuf to avoid a runtime dependency, and `.bin` snapshot fixtures. The recorded fixtures in `internal/testdata/` are derived from fli's captured responses, including the `GetBookingResults` capture behind `booking_results_aa_jfk_lax.txt`. |
| [krisukox/google-flights-api](https://github.com/krisukox/google-flights-api) (MIT) | Go ergonomics prior art and the `tfs` protobuf field numbers. Also a cautionary tale — its HTML scraping and browser-cookie dependency are why this client is RPC-only. |
| [AWeirdDev/flights](https://github.com/AWeirdDev/flights) (MIT) | The cleanest `flights.proto` for the `tfs` deep-link parameter. |
| [MomoDeve's tfs gist](https://gist.github.com/MomoDeve/a18053dea84dd28e320b8b2c489540eb) | The authoritative `tfs` field-number table, including fields the libraries above omit. |
| [ayushsaraswat.com writeup](https://ayushsaraswat.com/writing/reverse-engineering-google-flights/) | Response index gotchas and anti-bot context. |

Field maps we maintain from this work live in [`docs/wire/`](wire/).
