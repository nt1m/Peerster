package main

import (
  "fmt"
  "flag"
  "time"
  "github.com/dedis/protobuf"
  . "Peerster/types"
  . "Peerster/webserver"
)

var (
  UIPort = flag.String("UIPort", "8080",
    "port for the UI client")
  gossipAddr = flag.String("gossipAddr", "127.0.0.1:5000",
    "ip:port for the gossiper")
  name = flag.String("name", "300358",
    "name of the gossiper")
  peers = flag.String("peers", "127.0.0.1:5001,10.1.1.7:5002",
    "comma separated list of peers of the form ip:port")
  simpleMode = flag.Bool("simple", false,
    "run gossiper in simple broadcast mode")
)

func main() {
  flag.Parse()

  clientChannel := make(chan bool)
  localChannel := make(chan bool)

  antiEntropy := time.NewTicker(time.Second)
  client := NewClient("127.0.0.1:" + *UIPort)
  gossiper := NewGossiper(*gossipAddr, *name, *peers)

  go NewWebServer(*UIPort, gossiper)

  for {
    go handleClientMessages(gossiper, client, clientChannel)
    go handleServerMessages(gossiper, localChannel)
    select {
    case <-clientChannel:
      break;
    case <-localChannel:
      break;
    case <-antiEntropy.C:
      go (func() {
        random := gossiper.RandomPeer(gossiper.LastInteraction)
        gossiper.SendPacket(random, &GossipPacket{nil, nil, gossiper.GetStatusPacket()})
      })()
      break;
    }
  }
}


func handleServerMessages(gossiper *Gossiper, c chan bool) {
  packetBytes := make([]byte, 4096)
  var packet GossipPacket

  n, sender, _ := gossiper.Conn.ReadFromUDP(packetBytes)
  protobuf.Decode(packetBytes[:n], &packet)
  gossiper.AddPeer(sender)

  fmt.Println("PEERS", gossiper.PeersAsString())

  if packet.Simple != nil {
    packet.Simple.RelayPeerAddr = sender.String()
    packet.Simple.Log()
    gossiper.ForwardToAllPeers(sender, packetBytes)
  }

  if packet.Rumor != nil {
    // Ignore message if arrived in non-linear order
    if gossiper.ShouldIgnoreRumor(packet.Rumor) {
      c <- false
      return
    }
    packet.Rumor.Log(sender.String())

    // Forward the message if new
    if gossiper.IsNewRumor(packet.Rumor) {
      gossiper.RecordMessage(packet.Rumor)
      // Exclude sender, as they just sent it to us.
      gossiper.MongerMessage(packet.Rumor, sender, false)
    }
    gossiper.LastInteraction = sender
    gossiper.LastMessage[sender.String()] = packet.Rumor
    gossiper.SendPacket(sender, &GossipPacket{
      nil,
      nil,
      gossiper.GetStatusPacket(),
    })
  }

  if packet.Status != nil {
    if gossiper.Timeouts[sender.String()] != nil {
      close(gossiper.Timeouts[sender.String()])
      gossiper.Timeouts[sender.String()] = nil
    }

    packet.Status.Log(sender.String())

    newMessage := gossiper.GetNewMessageForPeer(packet.Status)
    if newMessage != nil {
      // Do I have a new message for other peer ? Yes, spread it
      gossiper.MongerMessage(newMessage, nil, false)
    } else if gossiper.PeerHasMessages(packet.Status) {
      // Does peer have new messages ? Yes, notify the sender of status
      gossiper.SendPacket(sender, &GossipPacket{nil, nil, gossiper.GetStatusPacket()})
    } else {
      fmt.Println("IN SYNC WITH", sender.String())
      // No, do a coin flip
      go gossiper.CoinFlip(gossiper.LastMessage[sender.String()], sender)
    }
  }

  c <- true
}

func handleClientMessages(gossiper *Gossiper, client *Client, c chan bool) {
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
    rumor := &RumorMessage{
      gossiper.Name,
      gossiper.GetNextIDForOrigin(gossiper.Name),
      msg.Text,
    }
    gossiper.RecordMessage(rumor)
    gossiper.MongerMessage(rumor, nil, false)
  }
  fmt.Println("CLIENT MESSAGE", msg.Text)
  c <- true
}
