package main

import (
  "fmt"
  "flag"
  "time"
  "net"
  "github.com/dedis/protobuf"
  "github.com/nt1m/Peerster/utils"
  . "github.com/nt1m/Peerster/types"
  . "github.com/nt1m/Peerster/webserver"
)

type PacketResult struct {
  packet *GossipPacket
  sender *net.UDPAddr
}

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

  clientChannel := make(chan Message)
  localChannel := make(chan PacketResult)

  antiEntropy := time.NewTicker(time.Second)
  client := NewClient("127.0.0.1:" + *UIPort)
  gossiper := NewGossiper(*gossipAddr, *name, *peers)

  go NewWebServer(*UIPort, gossiper)

  if !*simpleMode {
    gossiper.SendRouteMessage()
  }

  go receiveClientMessage(client, clientChannel)
  go receiveServerMessage(gossiper, localChannel)

  if (*rtimer > 0) {
    rticker = time.NewTicker(time.Duration(*rtimer) * time.Second).C
  }

  for {
    select {
    case msg := <-clientChannel:
      go receiveClientMessage(client, clientChannel)
      handleClientMessage(gossiper, client, &msg)
      break
    case received := <-localChannel:
      go receiveServerMessage(gossiper, localChannel)
      handleServerMessage(gossiper, received.packet, received.sender)
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

func receiveServerMessage(gossiper *Gossiper, c chan PacketResult) {
  packetBytes := make([]byte, 4096)
  var packet GossipPacket
  n, sender, err := gossiper.Conn.ReadFromUDP(packetBytes)
  utils.CheckError(err)
  protobuf.Decode(packetBytes[:n], &packet)
  c <- PacketResult{&packet, sender}
}

func receiveClientMessage(client *Client, c chan Message) {
  buf := make([]byte, 4096)
  var msg Message
  fmt.Println("Waiting for client message...")
  client.Conn.ReadFromUDP(buf)
  protobuf.Decode(buf, &msg)
  c <- msg
}

func handleServerMessage(gossiper *Gossiper, packet *GossipPacket, sender *net.UDPAddr) {
  gossiper.AddPeer(sender)

  fmt.Println("PEERS", gossiper.PeersAsString())

  if packet.Simple != nil {
    packet.Simple.RelayPeerAddr = sender.String()
    packet.Simple.Log()
    gossiper.ForwardToAllPeers(sender, packet)
  }

  if packet.Rumor != nil {
    gossiper.UpdateRoute(sender, packet.Rumor)
    // Ignore message if arrived in non-linear order
    if gossiper.ShouldIgnoreRumor(packet.Rumor) {
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
}

func handleClientMessage(gossiper *Gossiper, client *Client, msg *Message) {
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
    gossiper.ForwardToAllPeers(gossiper.Address, packet)
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
}
