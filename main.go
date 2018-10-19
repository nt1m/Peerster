package main

import (
  "fmt"
  "flag"
  "time"
  "github.com/dedis/protobuf"
  "github.com/nt1m/Peerster/utils"
  . "github.com/nt1m/Peerster/types"
  . "github.com/nt1m/Peerster/webserver"
)

var (
  UIPort = flag.String("UIPort", "8080",
    "port for the UI client")
  gossipAddr = flag.String("gossipAddr", "127.0.0.1:5000",
    "ip:port for the gossiper")
  name = flag.String("name", "300358",
    "name of the gossiper")
  peers = flag.String("peers", "127.0.0.1:5001",
    "comma separated list of peers of the form ip:port")
  simpleMode = flag.Bool("simple", false,
    "run gossiper in simple broadcast mode")
  rtimer = flag.Int("rtimer", 0,
    "route rumors sending period in seconds, 0 to disable sending of route rumors")
)

func main() {
  flag.Parse()

  var rticker (<-chan time.Time)

  clientChannel := make(chan bool)
  localChannel := make(chan bool)

  antiEntropy := time.NewTicker(time.Second)
  client := NewClient("127.0.0.1:" + *UIPort)
  gossiper := NewGossiper(*gossipAddr, *name, *peers)

  go NewWebServer(*UIPort, gossiper)

  // Figure out why this fucks test_2_ring.sh
  go gossiper.SendRouteMessage()

  go handleClientMessages(gossiper, client, clientChannel)
  go handleServerMessages(gossiper, localChannel)

  if (*rtimer > 0) {
    rticker = time.NewTicker(time.Duration(*rtimer) * time.Second).C
  }

  for {
    select {
    case <-clientChannel:
      go handleClientMessages(gossiper, client, clientChannel)
      break
    case <-localChannel:
      go handleServerMessages(gossiper, localChannel)
      break
    case <-antiEntropy.C:
      go (func() {
        random := gossiper.RandomPeer(gossiper.LastInteraction)
        gossiper.SendPacket(random, &GossipPacket{nil, nil, gossiper.GetStatusPacket(), nil})
        gossiper.LastInteraction = random
      })()
      break
    case <-rticker:
      go gossiper.SendRouteMessage()
      break
    }
  }
}


func handleServerMessages(gossiper *Gossiper, c chan bool) {
  packetBytes := make([]byte, 4096)
  var packet GossipPacket

  fmt.Println("Waiting for server message...")
  n, sender, err := gossiper.Conn.ReadFromUDP(packetBytes)
  utils.CheckError(err)
  protobuf.Decode(packetBytes[:n], &packet)
  gossiper.AddPeer(sender)

  fmt.Println("PEERS", gossiper.PeersAsString())

  if packet.Simple != nil {
    packet.Simple.RelayPeerAddr = sender.String()
    packet.Simple.Log()
    gossiper.ForwardToAllPeers(sender, packetBytes)
  }

  if packet.Rumor != nil {
    gossiper.UpdateRoute(sender, packet.Rumor)
    // Ignore message if arrived in non-linear order
    if gossiper.ShouldIgnoreRumor(packet.Rumor) {
      c <- false
      return
    }

    packet.Rumor.Log(sender.String())

    // Forward the message if new
    if gossiper.IsNewRumor(packet.Rumor) {
      gossiper.RecordRumor(packet.Rumor)
      // Exclude sender, as they just sent it to us.
      gossiper.MongerRumor(packet.Rumor, sender, false)
    }
    gossiper.LastInteraction = sender
    gossiper.LastRumor[sender.String()] = packet.Rumor
    gossiper.SendPacket(sender, &GossipPacket{
      nil,
      nil,
      gossiper.GetStatusPacket(),
      nil,
    })
  }

  if packet.Status != nil {
    if gossiper.Timeouts[sender.String()] != nil {
      close(gossiper.Timeouts[sender.String()])
      gossiper.Timeouts[sender.String()] = nil
    }

    packet.Status.Log(sender.String())

    newMessage := gossiper.GetNewRumorForPeer(packet.Status)
    if newMessage != nil {
      // Do I have a new message for other peer ? Yes, spread it
      gossiper.MongerRumor(newMessage, nil, false)
    } else if gossiper.PeerHasRumors(packet.Status) {
      // Does peer have new messages ? Yes, notify the sender of status
      gossiper.SendPacket(sender, &GossipPacket{nil, nil, gossiper.GetStatusPacket(), nil})
    } else {
      fmt.Println("IN SYNC WITH", sender.String())
      // No, do a coin flip
      go gossiper.CoinFlip(gossiper.LastRumor[sender.String()], sender)
    }
  }

  if packet.Private != nil {
    pm := packet.Private
    fmt.Println("Received private message", pm)
    if pm.Destination == gossiper.Name {
      pm.Log()
      gossiper.RecordPrivate(pm)
    } else {
      pm.HopLimit--
      gossiper.ForwardPrivate(pm)
    }
  }

  c <- true
}

func handleClientMessages(gossiper *Gossiper, client *Client, c chan bool) {
  buf := make([]byte, 4096)
  var msg Message
  fmt.Println("Waiting for client message...")
  client.Conn.ReadFromUDP(buf)
  protobuf.Decode(buf, &msg)

  if msg.Destination != "" {
    privateMessage := &PrivateMessage{
      Origin: gossiper.Name,
      ID: 0,
      Text: msg.Text,
      Destination: msg.Destination,
      HopLimit: 10,
    }
    gossiper.ForwardPrivate(privateMessage)
  } else if *simpleMode {
    packet := &GossipPacket{
      &SimpleMessage{
        gossiper.Name,
        *gossipAddr,
        msg.Text,
      },
      nil,
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
    gossiper.RecordRumor(rumor)
    gossiper.MongerRumor(rumor, nil, false)
  }
  fmt.Println("CLIENT MESSAGE", msg.Text)
  c <- true
}
