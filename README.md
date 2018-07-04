# Go DICOM parser

The Go DICOM parser is a library to parse DICOM files.

## Getting Started

### Installing
To start using, install Go 1.8 or above and run `go get`:
```sh
go get github.com/googlecloudplatform/go-dicom-parser
```
This will download the library source code into your `$GOPATH`

### Examples

```go
package main

import (
  "log"
  "os"
  "fmt"
  "github.com/googlecloudplatform/go-dicom-parser/dicom"
)

func main() {
  r, err := os.Open("dicomfile.dcm")
  if err != nil {
    log.Fatalf("os.Open(_) => %v", err)
  }
  dataSet := dicom.Parse(r)

  for tag, element := range dataSet.Elements {
    fmt.Println(tag, element.VR, element.ValueField)
  }
}

```

For more examples on library usage please visit the godoc
https://godoc.org/github.com/googlecloudplatform/go-dicom-parser

