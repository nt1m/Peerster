package main

import "fmt"
import "flag"
import "net"
import "github.com/dedis/protobuf"

type Message struct {
  Text string
  Destination string
}

func main() {
  var UIPort = flag.String("UIPort", "8080",
    "port for the UI client")
  var msgstr = flag.String("msg", "yo what's up",
    "message to be sent")
  var dest = flag.String("dest", "",
    "destination for the private message")
  flag.Parse()

  msg := Message{Text: *msgstr, Destination: *dest}
  packetBytes, err := protobuf.Encode(&msg)
  conn, err := net.Dial("udp4", "127.0.0.1:" + *UIPort)
  conn.Write(packetBytes)
  if (err != nil) {
    fmt.Println(err)
  }
}
