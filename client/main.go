package main

import _ "fmt"
import "flag"
import "net"
import "github.com/dedis/protobuf"

type Message struct {
  Text string
  Destination string
  File string
  Request string
}

func main() {
  var UIPort = flag.String("UIPort", "8080",
    "port for the UI client")
  var msgstr = flag.String("msg", "",
    "message to be sent")
  var dest = flag.String("dest", "",
    "destination for the private message")
  var file = flag.String("file", "", "file to be indexed by the gossiper")
  var request = flag.String("request", "", "request a chunk or metafile of this hash")
  flag.Parse()

  msg := Message{Text: *msgstr, Destination: *dest, File: *file, Request: *request}
  packetBytes, err := protobuf.Encode(&msg)
  checkError(err)
  conn, err := net.Dial("udp4", "127.0.0.1:" + *UIPort)
  checkError(err)
  conn.Write(packetBytes)
}

func checkError(err error) {
  if (err != nil) {
    panic(err)
  }
}
