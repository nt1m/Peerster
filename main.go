package main

import (
  "fmt"
  "flag"
  "net"
  "github.com/dedis/protobuf"
  . "Peerster/types"
  "Peerster/utils"
)

var (
  UIPort = flag.String("UIPort", "8080",
    "port for the UI client")
  gossipAddr = flag.String("gossipAddr", "127.0.0.1:5000",
    "ip:port for the gossiper")
  name = flag.String("name", "nodeA",
    "name of the gossiper")
  peers = flag.String("peers", "127.0.0.1:5001,10.1.1.7:5002",
    "comma separated list of peers of the form ip:port")
  simple = flag.Bool("simple", true,
    "run gossiper in simple broadcast mode")
)

func main() {
  flag.Parse()

  fmt.Println("Simple", *simple)

  clientChannel := make(chan Message);
  localChannel := make(chan GossipPacket);

  client := NewClient("127.0.0.1:" + *UIPort)
  receiver := NewGossiper(*gossipAddr, *name, *peers)

  for {
    go handleClientMessages(receiver, client, clientChannel);
    go handleServerMessages(receiver, localChannel);
    select {
    case msg := <-clientChannel:
      fmt.Println("CLIENT MESSAGE", msg.Text);
      break;
    case packet := <-localChannel:
      fmt.Println("SIMPLE MESSAGE origin", packet.Simple.OriginalName, "from", packet.Simple.RelayPeerAddr, "contents", packet.Simple.Contents);
      break;
    }
  }
}

func handleServerMessages(receiver *Gossiper, c chan GossipPacket) {
  buf := make([]byte, 4096)
  var packet GossipPacket

  n, sender, _ := receiver.Conn.ReadFromUDP(buf)
  protobuf.Decode(buf[:n], &packet)
  receiver.AddPeer(sender)

  fmt.Println("PEERS", receiver.PeersAsString())

  packetBytes, err := protobuf.Encode(&packet)
  utils.CheckError(err)

  if packet.Simple != nil {
    packet.Simple.RelayPeerAddr = sender.String()
    forwardToAllPeers(receiver, sender, packetBytes)
  }

  if packet.Rumor != nil {
    // XXX: Forward the message if new
    if receiver.IsNewRumor(packet.Rumor) {
      receiver.UpdateStatus(packet.Rumor)
      receiver.Conn.WriteToUDP(packetBytes, receiver.RandomPeer())
    }
  
  }

  if packet.Status != nil {
    // XXX: Status

  }

  c <- packet
}


func forwardToAllPeers(gossiper *Gossiper, sender *net.UDPAddr, packetBytes []byte) {
  for _,peer := range gossiper.Peers {
    // Don't send to the person who just sent to you
    if (peer.String() == sender.String()) {
      return;
    }
    gossiper.Conn.WriteToUDP(packetBytes, peer)
  }
}

func handleClientMessages(gossiper *Gossiper, client *Client, c chan Message) {
  buf := make([]byte, 4096)
  var msg Message
  client.Conn.ReadFromUDP(buf)
  protobuf.Decode(buf, &msg)


  packet := &GossipPacket{
    &SimpleMessage{
      gossiper.Name,
      *gossipAddr,
      msg.Text,
    },
    nil,
    nil,
  }
  packetBytes, err := protobuf.Encode(packet)

  utils.CheckError(err)
  forwardToAllPeers(gossiper, gossiper.Address, packetBytes)
  c <- msg
}
