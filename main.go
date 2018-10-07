package main

import (
  "fmt"
  "flag"
  "time"
  "github.com/dedis/protobuf"
  . "Peerster/types"
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
  simpleMode = flag.Bool("simple", false,
    "run gossiper in simple broadcast mode")
)

func main() {
  flag.Parse()

  fmt.Println("Running with -simple=", *simpleMode)

  clientChannel := make(chan Message)
  localChannel := make(chan GossipPacket)

  antiEntropy := time.NewTicker(time.Second)
  client := NewClient("127.0.0.1:" + *UIPort)
  receiver := NewGossiper(*gossipAddr, *name, *peers)

  for {
    go handleClientMessages(receiver, client, clientChannel)
    go handleServerMessages(receiver, localChannel)
    select {
    case <-clientChannel:
      break;
    case <-localChannel:
      break;
    case <-antiEntropy.C:
      go (func() {
        random := receiver.RandomPeer(nil)
        receiver.SendPacket(random, &GossipPacket{nil, nil, receiver.GetStatusPacket()})
      })()
      break;
    }
  }
}

func handleServerMessages(receiver *Gossiper, c chan GossipPacket) {
  packetBytes := make([]byte, 4096)
  var packet GossipPacket

  n, sender, _ := receiver.Conn.ReadFromUDP(packetBytes)
  protobuf.Decode(packetBytes[:n], &packet)
  receiver.AddPeer(sender)

  fmt.Println("PEERS", receiver.PeersAsString())

  if packet.Simple != nil {
    packet.Simple.RelayPeerAddr = sender.String()
    packet.Simple.Log()
    receiver.ForwardToAllPeers(sender, packetBytes)
  }

  if packet.Rumor != nil {
    // XXX: ignore message if arrived in non-linear order
    packet.Rumor.Log(sender.String())
    // Forward the message if new
    if receiver.IsNewRumor(packet.Rumor) {
      receiver.RecordMessage(packet.Rumor)
      receiver.MongerMessage(packet.Rumor, nil, false)
    }

    statusPacket := &GossipPacket{
      nil,
      nil,
      receiver.GetStatusPacket(),
    }
    receiver.SendPacket(sender, statusPacket)
  }

  if packet.Status != nil {
    if receiver.Timeouts[sender.String()] != nil {
      close(receiver.Timeouts[sender.String()])
      receiver.Timeouts[sender.String()] = nil
    }

    packet.Status.Log(sender.String())

    newMessage := receiver.GetNewMessageForPeer(packet.Status)
    if newMessage != nil {
      fmt.Println("IN SYNC WITH", sender.String())
      // Do I have a new message for other peer ? Yes, spread it
      receiver.MongerMessage(newMessage, nil, false)
    } else if receiver.PeerHasMessages(packet.Status) {
      // Does peer have new messages ? Yes, notify the sender of status
      receiver.SendPacket(sender, &GossipPacket{nil, nil, receiver.GetStatusPacket()})
    } else {
      fmt.Println("IN SYNC WITH", sender.String())
      // No, do a coin flip
      // go receiver.CoinFlip(sender)
    }
  }

  c <- packet
}

func handleClientMessages(gossiper *Gossiper, client *Client, c chan Message) {
  buf := make([]byte, 4096)
  var msg Message
  client.Conn.ReadFromUDP(buf)
  protobuf.Decode(buf, &msg)

  if *simpleMode {
    packet := &GossipPacket{
      &SimpleMessage{
        gossiper.Name,
        *gossipAddr,
        msg.Text,
      },
      nil,
      nil,
    }
    packetBytes := EncodePacket(packet)
    gossiper.ForwardToAllPeers(gossiper.Address, packetBytes)
  } else {
    gossiper.MongerMessage(&RumorMessage{
      gossiper.Name,
      gossiper.CurrentID,
      msg.Text,
    }, nil, false)
    gossiper.CurrentID = gossiper.CurrentID + 1
  }
  fmt.Println("CLIENT MESSAGE", msg.Text)
  c <- msg
}
