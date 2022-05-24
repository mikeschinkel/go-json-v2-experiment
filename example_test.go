// Copyright 2022 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package json_test

import (
	"bytes"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/netip"
	"reflect"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-json-experiment/json"
)

// If a type implements encoding.TextMarshaler and/or encoding.TextUnmarshaler,
// then the MarshalText and UnmarshalText methods are used to encode/decode
// the value to/from a JSON string.
func Example_textMarshal() {
	// Round-trip marshal and unmarshal a hostname map where the netip.Addr type
	// implements both encoding.TextMarshaler and encoding.TextUnmarshaler.
	want := map[netip.Addr]string{
		netip.MustParseAddr("192.168.0.100"): "carbonite",
		netip.MustParseAddr("192.168.0.101"): "obsidian",
		netip.MustParseAddr("192.168.0.102"): "diamond",
	}
	b, err := json.Marshal(&want)
	if err != nil {
		log.Fatal(err)
	}
	var got map[netip.Addr]string
	err = json.Unmarshal(b, &got)
	if err != nil {
		log.Fatal(err)
	}

	// Sanity check.
	if !reflect.DeepEqual(got, want) {
		log.Fatalf("roundtrip mismatch: got %v, want %v", got, want)
	}

	// Print the serialized JSON object. Canonicalize the JSON first since
	// Go map entries are not serialized in a deterministic order.
	(*json.RawValue)(&b).Canonicalize()
	(*json.RawValue)(&b).Indent("", "\t") // indent for readability
	fmt.Println(string(b))

	// Output:
	// {
	// 	"192.168.0.100": "carbonite",
	// 	"192.168.0.101": "obsidian",
	// 	"192.168.0.102": "diamond"
	// }
}

// By default, JSON object names for Go struct fields are derived from
// the Go field name, but may be specified in the `json` tag.
// Due to JSON's heritage in JavaScript, the most common naming convention
// used for JSON object names is camelCase.
func Example_fieldNames() {
	var value struct {
		// This field is explicitly ignored with the special "-" name.
		Ignored any `json:"-"`
		// No JSON name is not provided, so the Go field name is used.
		GoName any
		// A JSON name is provided without any special characters.
		JSONName any `json:"jsonName"`
		// No JSON name is not provided, so the Go field name is used.
		Option any `json:",nocase"`
		// An empty JSON name specified using an single-quoted string literal.
		Empty any `json:"''"`
		// A dash JSON name specified using an single-quoted string literal.
		Dash any `json:"'-'"`
		// A comma JSON name specified using an single-quoted string literal.
		Comma any `json:"','"`
		// JSON name with quotes specified using a single-quoted string literal.
		Quote any `json:"'\"\\''"`
		// An unexported field is always ignored.
		unexported any
	}

	b, err := json.Marshal(value)
	if err != nil {
		log.Fatal(err)
	}
	(*json.RawValue)(&b).Indent("", "\t") // indent for readability
	fmt.Println(string(b))

	// Output:
	// {
	// 	"GoName": null,
	// 	"jsonName": null,
	// 	"Option": null,
	// 	"": null,
	// 	"-": null,
	// 	",": null,
	// 	"\"'": null
	// }
}

// Unmarshal matches JSON object names with Go struct fields using
// a case-sensitive match, but can be configured to use a case-insensitive
// match with the "nocase" option. This permits unmarshaling from inputs that
// use naming conventions such as camelCase, snake_case, or kebab-case.
func Example_caseSensitivity() {
	// JSON input using various naming conventions.
	const input = `[
		{"firstname": true},
		{"firstName": true},
		{"FirstName": true},
		{"FIRSTNAME": true},
		{"first_name": true},
		{"FIRST_NAME": true},
		{"first-name": true},
		{"FIRST-NAME": true},
		{"unknown": true}
	]`

	// Without "nocase", Unmarshal looks for an exact match.
	var withcase []struct {
		X bool `json:"firstName"`
	}
	if err := json.Unmarshal([]byte(input), &withcase); err != nil {
		log.Fatal(err)
	}
	fmt.Println(withcase) // exactly 1 match found

	// With "nocase", Unmarshal looks first for an exact match,
	// then for a case-insensitive match if none found.
	var nocase []struct {
		X bool `json:"firstName,nocase"`
	}
	if err := json.Unmarshal([]byte(input), &nocase); err != nil {
		log.Fatal(err)
	}
	fmt.Println(nocase) // 8 matches found

	// Output:
	// [{false} {true} {false} {false} {false} {false} {false} {false} {false}]
	// [{true} {true} {true} {true} {true} {true} {true} {true} {false}]
}

// JSON objects can be inlined within a parent object similar to
// how Go structs can be embedded within a parent struct.
// The inlining rules are similar to those of Go embedding,
// but operates upon the JSON namespace.
func Example_inlinedFields() {
	// Base is embedded within Container.
	type Base struct {
		// ID is promoted into the JSON object for Container.
		ID string
		// Type is ignored due to presence of Container.Type.
		Type string
		// Time cancels out with Container.Inlined.Time.
		Time time.Time
	}
	// Other is embedded within Container.
	type Other struct{ Cost float64 }
	// Container embeds Base and Other.
	type Container struct {
		// Base is an embedded struct and is implicitly JSON inlined.
		Base
		// Type takes precedence over Base.Type.
		Type int
		// Inlined is a named Go field, but is explicitly JSON inlined.
		Inlined struct {
			// User is promoted into the JSON object for Container.
			User string
			// Time cancels out with Base.Time.
			Time string
		} `json:",inline"`
		// ID does not conflict with Base.ID since the JSON name is different.
		ID string `json:"uuid"`
		// Other is not JSON inlined since it has an explicit JSON name.
		Other `json:"other"`
	}

	// Format an empty Container to show what fields are JSON serializable.
	var input Container
	b, err := json.Marshal(&input)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(b))

	// Output:
	// {"ID":"","Type":0,"User":"","uuid":"","other":{"Cost":0}}
}

// The "format" tag option can be used to alter the formatting of certain types.
func Example_formatFlags() {
	value := struct {
		BytesBase64    []byte         `json:",format:base64"`
		BytesHex       [8]byte        `json:",format:hex"`
		BytesArray     []byte         `json:",format:array"`
		FloatNonFinite float64        `json:",format:nonfinite"`
		MapEmitNull    map[string]any `json:",format:emitnull"`
		SliceEmitNull  []any          `json:",format:emitnull"`
		TimeDateOnly   time.Time      `json:",format:'2006-01-02'"`
		DurationNanos  time.Duration  `json:",format:nanos"`
	}{
		BytesBase64:    []byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef},
		BytesHex:       [8]byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef},
		BytesArray:     []byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef},
		FloatNonFinite: math.NaN(),
		MapEmitNull:    nil,
		SliceEmitNull:  nil,
		TimeDateOnly:   time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
		DurationNanos:  time.Second + time.Millisecond + time.Microsecond + time.Nanosecond,
	}

	b, err := json.Marshal(&value)
	if err != nil {
		log.Fatal(err)
	}
	(*json.RawValue)(&b).Indent("", "\t") // indent for readability
	fmt.Println(string(b))

	// Output:
	// {
	// 	"BytesBase64": "ASNFZ4mrze8=",
	// 	"BytesHex": "0123456789abcdef",
	// 	"BytesArray": [
	// 		1,
	// 		35,
	// 		69,
	// 		103,
	// 		137,
	// 		171,
	// 		205,
	// 		239
	// 	],
	// 	"FloatNonFinite": "NaN",
	// 	"MapEmitNull": null,
	// 	"SliceEmitNull": null,
	// 	"TimeDateOnly": "2000-01-01",
	// 	"DurationNanos": 1001001001
	// }
}

// When implementing HTTP endpoints, it is common to be operating with an
// io.Reader and an io.Writer. The UnmarshalFull and MarshalFull functions
// assist in operating on such input/output types.
// UnmarshalFull reads the entirety of the io.Reader to ensure that io.EOF
// is encountered without any unexpected bytes after the top-level JSON value.
func Example_serveHTTP() {
	// Some global state maintained by the server.
	var n int64

	// The "add" endpoint accepts a POST request with a JSON object
	// containing a number to atomically add to the server's global counter.
	// It returns the updated value of the counter.
	http.HandleFunc("/api/add", func(w http.ResponseWriter, r *http.Request) {
		// Unmarshal the request from the client.
		var val struct{ N int64 }
		if err := json.UnmarshalFull(r.Body, &val); err != nil {
			// Inability to unmarshal the input suggests a client-side problem.
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Marshal a response from the server.
		val.N = atomic.AddInt64(&n, val.N)
		if err := json.MarshalFull(w, &val); err != nil {
			// Inability to marshal the output suggests a server-side problem.
			// This error is not always observable by the client since
			// json.MarshalFull may have already written to the output.
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
}

// Directly embedding JSON within HTML requires special handling for safety.
// Escape certain runes to prevent JSON directly treated as HTML
// from being able to perform <script> injection.
//
// This example shows how to obtain equivalent behavior provided by the
// "encoding/json" package that is no longer directly supported by this package.
// Newly written code that intermix JSON and HTML should instead be using the
// "github.com/google/safehtml" module for safety purposes.
func ExampleEncodeOptions_escapeHTML() {
	page := struct {
		Title string
		Body  string
	}{
		Title: "Example Embedded Javascript",
		Body:  `<script> console.log("Hello, world!"); </script>`,
	}

	b, err := json.MarshalOptions{}.Marshal(json.EncodeOptions{
		// Escape certain runes within a JSON string so that
		// JSON will be safe to directly embed inside HTML.
		EscapeRune: func(r rune) bool {
			switch r {
			case '&', '<', '>', '\u2028', '\u2029':
				return true
			default:
				return false
			}
		},
		// Indent the output for readability.
		Indent: "\t",
	}, &page)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(b))

	// Output:
	// {
	// 	"Title": "Example Embedded Javascript",
	// 	"Body": "\u003cscript\u003e console.log(\"Hello, world!\"); \u003c/script\u003e"
	// }
}

// In some applications, the exact precision of JSON numbers needs to be
// preserved when unmarshaling. This can be accomplished using a type-specific
// unmarshal function that intercepts all any types and pre-populates the
// interface value with a RawValue, which can represent a JSON number exactly.
func ExampleUnmarshalOptions_rawNumber() {
	// Input with JSON numbers beyond the representation of a float64.
	const input = `[false, 1e-1000, 3.141592653589793238462643383279, 1e+1000, true]`

	var value any
	err := json.UnmarshalOptions{
		// Intercept every attempt to unmarshal into the any type.
		Unmarshalers: json.UnmarshalFuncV2(func(opts json.UnmarshalOptions, dec *json.Decoder, val *any) error {
			// If the next value to be decoded is a JSON number,
			// then provide a concrete Go type to unmarshal into.
			if dec.PeekKind() == '0' {
				*val = json.RawValue(nil)
			}
			// Return SkipFunc to fallback on default unmarshal behavior.
			return json.SkipFunc
		}),
	}.Unmarshal(json.DecodeOptions{}, []byte(input), &value)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(value)

	// Sanity check.
	want := []any{false, json.RawValue("1e-1000"), json.RawValue("3.141592653589793238462643383279"), json.RawValue("1e+1000"), true}
	if !reflect.DeepEqual(value, want) {
		log.Fatalf("value mismatch:\ngot  %v\nwant %v", value, want)
	}

	// Output:
	// [false 1e-1000 3.141592653589793238462643383279 1e+1000 true]
}

// When using JSON for parsing configuration files,
// the parsing logic often needs to report an error with a line and column
// indicating where in the input an error occurred.
func ExampleUnmarshalOptions_recordOffsets() {
	// Hypothetical configuration file.
	const input = `[
		{"Source": "192.168.0.100:1234", "Destination": "192.168.0.1:80"},
		{"Source": "192.168.0.251:4004"},
		{"Source": "192.168.0.165:8080", "Destination": "0.0.0.0:80"}
	]`
	type Tunnel struct {
		Source      netip.AddrPort
		Destination netip.AddrPort

		// ByteOffset is populated during unmarshal with the byte offset
		// within the JSON input of the JSON object for this Go struct.
		ByteOffset int64 `json:"-"` // metadata to be ignored for JSON serialization
	}

	var tunnels []Tunnel
	err := json.UnmarshalOptions{
		// Intercept every attempt to unmarshal into the Tunnel type.
		Unmarshalers: json.UnmarshalFuncV2(func(opts json.UnmarshalOptions, dec *json.Decoder, tunnel *Tunnel) error {
			// Decoder.InputOffset reports the offset after the last token,
			// but we want to record the offset before the next token.
			//
			// Call Decoder.PeekKind to buffer enough to reach the next token.
			// Add the number of leading whitespace, commas, and colons
			// to locate the start of the next token.
			dec.PeekKind()
			unread := dec.UnreadBuffer()
			n := len(unread) - len(bytes.TrimLeft(unread, " \n\r\t,:"))
			tunnel.ByteOffset = dec.InputOffset() + int64(n)

			// Return SkipFunc to fallback on default unmarshal behavior.
			return json.SkipFunc
		}),
	}.Unmarshal(json.DecodeOptions{}, []byte(input), &tunnels)
	if err != nil {
		log.Fatal(err)
	}

	// lineColumn converts a byte offset into a one-indexed line and column.
	// The offset must be within the bounds of the input.
	lineColumn := func(input string, offset int) (line, column int) {
		line = 1 + strings.Count(input[:offset], "\n")
		column = 1 + offset - (strings.LastIndex(input[:offset], "\n") + len("\n"))
		return line, column
	}

	// Verify that the configuration file is valid.
	for _, tunnel := range tunnels {
		if !tunnel.Source.IsValid() || !tunnel.Destination.IsValid() {
			line, column := lineColumn(input, int(tunnel.ByteOffset))
			fmt.Printf("%d:%d: source and destination must both be specified", line, column)
		}
	}

	// Output:
	// 3:3: source and destination must both be specified
}
