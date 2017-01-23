// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package io

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"gopkg.in/check.v1"
)

type entry struct {
	Message string
	Source  string
}

type testFormatter struct{}

func (testFormatter) Format(out io.Writer, data []byte) error {
	var entries []entry
	err := json.Unmarshal(data, &entries)
	if err != nil {
		return ErrInvalidStreamChunk
	}
	for _, e := range entries {
		fmt.Fprintf(out, "%s-%s\n", e.Source, e.Message)
	}
	return nil
}

func (s *S) TestStreamWriterUsesFormatter(c *check.C) {
	entries := []entry{
		{Message: "Something happened", Source: "tsuru"},
		{Message: "Something happened again", Source: "tsuru"},
	}
	data, err := json.Marshal(entries)
	c.Assert(err, check.IsNil)
	var writer bytes.Buffer
	w := NewStreamWriter(&writer, testFormatter{})
	w.Write(data)
	expected := "tsuru-Something happened\ntsuru-Something happened again\n"
	c.Assert(writer.String(), check.Equals, expected)
	c.Assert(w.Remaining(), check.DeepEquals, []byte{})
}

func (s *S) TestStreamWriterChukedWrite(c *check.C) {
	entries := []entry{
		{Message: "\nSome\nthing\nhappened\n", Source: "tsuru"},
		{Message: "Something happened again", Source: "tsuru"},
	}
	data, err := json.Marshal(entries)
	c.Assert(err, check.IsNil)
	l := len(data)
	var buf bytes.Buffer
	w := NewStreamWriter(&buf, testFormatter{})
	_, err = w.Write(data[:l/4])
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "")
	_, err = w.Write(data[l/4 : l/2])
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "")
	_, err = w.Write(data[l/2 : l/4*3])
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "")
	_, err = w.Write(data[l/4*3:])
	c.Assert(err, check.IsNil)
	expected := "tsuru-\nSome\nthing\nhappened\n\ntsuru-Something happened again\n"
	c.Assert(buf.String(), check.Equals, expected)
	c.Assert(w.Remaining(), check.DeepEquals, []byte{})
}

func (s *S) TestStreamWriter(c *check.C) {
	entries := []entry{
		{Message: "Something happened", Source: "tsuru"},
		{Message: "Something happened again", Source: "tsuru"},
	}
	b, err := json.Marshal(entries)
	c.Assert(err, check.IsNil)
	var writer bytes.Buffer
	w := NewStreamWriter(&writer, testFormatter{})
	n, err := w.Write(b)
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, len(b))
	expected := "tsuru-Something happened\ntsuru-Something happened again\n"
	c.Assert(writer.String(), check.Equals, expected)
	c.Assert(w.Remaining(), check.DeepEquals, []byte{})
}

func (s *S) TestStreamWriterMultipleChunksOneMessage(c *check.C) {
	entries := []entry{
		{Message: "Something 1", Source: "tsuru"},
	}
	b, err := json.Marshal(entries)
	c.Assert(err, check.IsNil)
	var writer bytes.Buffer
	w := NewStreamWriter(&writer, testFormatter{})
	n, err := w.Write(append(b, append([]byte("\n"), b...)...))
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 2*len(b)+1)
	expected := "tsuru-Something 1\ntsuru-Something 1\n"
	c.Assert(writer.String(), check.Equals, expected)
	c.Assert(w.Remaining(), check.DeepEquals, []byte{})
}

func (s *S) TestStreamWriterInvalidDataNotRead(c *check.C) {
	entries := []entry{
		{Message: "Something 1", Source: "tsuru"},
	}
	b, err := json.Marshal(entries)
	c.Assert(err, check.IsNil)
	var writer bytes.Buffer
	w := NewStreamWriter(&writer, testFormatter{})
	toWrite := append(b, []byte("\ninvalid data")...)
	n, err := w.Write(toWrite)
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, len(toWrite))
	expected := "tsuru-Something 1\n"
	c.Assert(writer.String(), check.Equals, expected)
	c.Assert(w.Remaining(), check.DeepEquals, []byte("invalid data"))
}

func (s *S) TestStreamWriterInvalidDataNotReadInChunk(c *check.C) {
	entries := []entry{
		{Message: "Something 1", Source: "tsuru"},
	}
	b, err := json.Marshal(entries)
	c.Assert(err, check.IsNil)
	var writer bytes.Buffer
	w := NewStreamWriter(&writer, testFormatter{})
	toWrite := append(b, []byte("\ninvalid data\n")...)
	n, err := w.Write(toWrite)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Unparseable chunk: \"invalid data\\n\"")
	c.Assert(n, check.Equals, len(toWrite))
	expected := "tsuru-Something 1\n"
	c.Assert(writer.String(), check.Equals, expected)
	c.Assert(w.Remaining(), check.DeepEquals, []byte("invalid data\n"))
}

func (s *S) TestStreamWriterOnlyInvalidMessage(c *check.C) {
	var writer bytes.Buffer
	w := NewStreamWriter(&writer, testFormatter{})
	b := []byte("-----")
	n, err := w.Write(b)
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 5)
	c.Assert(writer.String(), check.Equals, "")
	c.Assert(w.Remaining(), check.DeepEquals, []byte("-----"))
}

func (s *S) TestStreamWriterOnlyInvalidMessageInChunk(c *check.C) {
	var writer bytes.Buffer
	w := NewStreamWriter(&writer, testFormatter{})
	b := []byte("-----\n")
	n, err := w.Write(b)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Unparseable chunk: \"-----\\n\"")
	c.Assert(n, check.Equals, 6)
	c.Assert(writer.String(), check.Equals, "")
	c.Assert(w.Remaining(), check.DeepEquals, []byte("-----\n"))
}

func (s *S) TestStreamWriterInvalidDataNotReadInMultipleChunks(c *check.C) {
	entries := []entry{
		{Message: "Something 1", Source: "tsuru"},
	}
	b, err := json.Marshal(entries)
	c.Assert(err, check.IsNil)
	var writer bytes.Buffer
	w := NewStreamWriter(&writer, testFormatter{})
	toWrite := append(b, []byte("\ninvalid data\nmoreinvalid\nsomething")...)
	n, err := w.Write(toWrite)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Unparseable chunk: \"invalid data\\n\"")
	c.Assert(n, check.Equals, len(toWrite))
	expected := "tsuru-Something 1\n"
	c.Assert(writer.String(), check.Equals, expected)
	c.Assert(w.Remaining(), check.DeepEquals, []byte("invalid data\nmoreinvalid\nsomething"))
}

func (s *S) TestSimpleJsonMessageFormatter(c *check.C) {
	formatter := SimpleJsonMessageFormatter{}
	buf := bytes.Buffer{}
	err := formatter.Format(&buf, []byte(`{"message": "mymsg"}`))
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "mymsg")
	buf = bytes.Buffer{}
	err = formatter.Format(&buf, []byte(`{"message": "mym`))
	c.Assert(err, check.Equals, ErrInvalidStreamChunk)
	c.Assert(buf.String(), check.Equals, "")
	buf = bytes.Buffer{}
	err = formatter.Format(&buf, []byte(`{"message": "mymsg", "error": "myerror"}`))
	c.Assert(err, check.ErrorMatches, "myerror")
	c.Assert(buf.String(), check.Equals, "")
}

var mockPullOutput = `{"status":"Pulling from tsuru/static","id":"latest"}
{"status":"Already exists","progressDetail":{},"id":"a6aa3b66376f"}
{"status":"Pulling fs layer","progressDetail":{},"id":"106572778bf7"}
{"status":"Pulling fs layer","progressDetail":{},"id":"bac681833e51"}
{"status":"Pulling fs layer","progressDetail":{},"id":"7302e23ef08a"}
{"status":"Downloading","progressDetail":{"current":621,"total":621},"progress":"[==================================================\u003e]    621 B/621 B","id":"bac681833e51"}
{"status":"Verifying Checksum","progressDetail":{},"id":"bac681833e51"}
{"status":"Download complete","progressDetail":{},"id":"bac681833e51"}
{"status":"Downloading","progressDetail":{"current":1854,"total":1854},"progress":"[==================================================\u003e] 1.854 kB/1.854 kB","id":"106572778bf7"}
{"status":"Verifying Checksum","progressDetail":{},"id":"106572778bf7"}
{"status":"Download complete","progressDetail":{},"id":"106572778bf7"}
{"status":"Extracting","progressDetail":{"current":1854,"total":1854},"progress":"[==================================================\u003e] 1.854 kB/1.854 kB","id":"106572778bf7"}
{"status":"Extracting","progressDetail":{"current":1854,"total":1854},"progress":"[==================================================\u003e] 1.854 kB/1.854 kB","id":"106572778bf7"}
{"status":"Downloading","progressDetail":{"current":233019,"total":21059403},"progress":"[\u003e                                                  ]   233 kB/21.06 MB","id":"7302e23ef08a"}
{"status":"Downloading","progressDetail":{"current":462395,"total":21059403},"progress":"[=\u003e                                                 ] 462.4 kB/21.06 MB","id":"7302e23ef08a"}
{"status":"Downloading","progressDetail":{"current":8490555,"total":21059403},"progress":"[====================\u003e                              ] 8.491 MB/21.06 MB","id":"7302e23ef08a"}
{"status":"Downloading","progressDetail":{"current":20876859,"total":21059403},"progress":"[=================================================\u003e ] 20.88 MB/21.06 MB","id":"7302e23ef08a"}
{"status":"Verifying Checksum","progressDetail":{},"id":"7302e23ef08a"}
{"status":"Download complete","progressDetail":{},"id":"7302e23ef08a"}
{"status":"Pull complete","progressDetail":{},"id":"106572778bf7"}
{"status":"Extracting","progressDetail":{"current":621,"total":621},"progress":"[==================================================\u003e]    621 B/621 B","id":"bac681833e51"}
{"status":"Extracting","progressDetail":{"current":621,"total":621},"progress":"[==================================================\u003e]    621 B/621 B","id":"bac681833e51"}
{"status":"Pull complete","progressDetail":{},"id":"bac681833e51"}
{"status":"Extracting","progressDetail":{"current":229376,"total":21059403},"progress":"[\u003e                                                  ] 229.4 kB/21.06 MB","id":"7302e23ef08a"}
{"status":"Extracting","progressDetail":{"current":458752,"total":21059403},"progress":"[=\u003e                                                 ] 458.8 kB/21.06 MB","id":"7302e23ef08a"}
{"status":"Extracting","progressDetail":{"current":11239424,"total":21059403},"progress":"[==========================\u003e                        ] 11.24 MB/21.06 MB","id":"7302e23ef08a"}
{"status":"Extracting","progressDetail":{"current":21059403,"total":21059403},"progress":"[==================================================\u003e] 21.06 MB/21.06 MB","id":"7302e23ef08a"}
{"status":"Pull complete","progressDetail":{},"id":"7302e23ef08a"}
{"status":"Digest: sha256:b754472891aa7e33fc0214e3efa988174f2c2289285fcae868b7ec8b6675fc77"}
{"status":"Status: Downloaded newer image for 192.168.50.4:5000/tsuru/static"}
`

func (s *S) TestSimpleJsonMessageFormatterJsonInJson(c *check.C) {
	buf := bytes.Buffer{}
	encoder := json.NewEncoder(&buf)
	writer := SimpleJsonMessageEncoderWriter{encoder}
	for _, l := range bytes.Split([]byte(mockPullOutput), []byte("\n")) {
		writer.Write(l)
	}
	parts := bytes.Split(buf.Bytes(), []byte("\n"))
	parts = append([][]byte{[]byte(`{"message":"no json 1\n"}`)}, parts...)
	parts = append(parts, []byte(`{"message":"no json 2\n"}`))
	outBuf := bytes.Buffer{}
	streamWriter := NewStreamWriter(&outBuf, nil)
	written, err := streamWriter.Write(bytes.Join(parts, []byte("\n")))
	c.Assert(err, check.IsNil)
	c.Assert(written, check.Equals, 4612)
	c.Assert(outBuf.String(), check.Equals, "no json 1\n"+
		"latest: Pulling from tsuru/static\n"+
		"a6aa3b66376f: Already exists\n"+
		"106572778bf7: Pulling fs layer\n"+
		"bac681833e51: Pulling fs layer\n"+
		"7302e23ef08a: Pulling fs layer\n"+
		"bac681833e51: Verifying Checksum\n"+
		"bac681833e51: Download complete\n"+
		"106572778bf7: Verifying Checksum\n"+
		"106572778bf7: Download complete\n"+
		"7302e23ef08a: Verifying Checksum\n"+
		"7302e23ef08a: Download complete\n"+
		"106572778bf7: Pull complete\n"+
		"bac681833e51: Pull complete\n"+
		"7302e23ef08a: Pull complete\n"+
		"Digest: sha256:b754472891aa7e33fc0214e3efa988174f2c2289285fcae868b7ec8b6675fc77\n"+
		"Status: Downloaded newer image for 192.168.50.4:5000/tsuru/static\n"+
		"no json 2\n")
}

func (s *S) TestSimpleJsonMessageEncoderWriter(c *check.C) {
	buf := bytes.Buffer{}
	encoder := json.NewEncoder(&buf)
	writer := SimpleJsonMessageEncoderWriter{encoder}
	written, err := writer.Write([]byte("my cool msg"))
	c.Assert(written, check.Equals, 11)
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, `{"Message":"my cool msg"}`+"\n")
}
