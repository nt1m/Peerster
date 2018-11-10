package main

import (
  "fmt"
  "flag"
  "time"
  "net"
  "os"
  "crypto/sha256"
  "encoding/hex"
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
        gossiper.SendPacket(random, &GossipPacket{nil, nil, gossiper.GetStatusPacket(), nil, nil, nil})
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
  packetBytes := make([]byte, 16384)
  var packet GossipPacket
  n, sender, err := gossiper.Conn.ReadFromUDP(packetBytes)
  utils.CheckError(err)
  protobuf.Decode(packetBytes[:n], &packet)
  c <- PacketResult{&packet, sender}
}

func receiveClientMessage(client *Client, c chan Message) {
  buf := make([]byte, 16384)
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
      nil,
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
      gossiper.SendPacket(sender, &GossipPacket{nil, nil, gossiper.GetStatusPacket(), nil, nil, nil})
    } else {
      fmt.Println("IN SYNC WITH", sender.String())
      // No, do a coin flip
      go gossiper.CoinFlip(gossiper.LastRumor[sender.String()], sender)
    }
  }

  if packet.Private != nil {
    pm := packet.Private
    if pm.Destination == gossiper.Name {
      pm.Log()
      gossiper.RecordPrivate(pm)
    } else {
      pm.HopLimit--
      gossiper.ForwardPrivate(pm)
    }
  }

  if packet.DataRequest != nil {
    rq := packet.DataRequest
    if rq.Destination == gossiper.Name {
      gossiper.ReplyDataRequest(rq)
    } else {
      rq.HopLimit--
      gossiper.ForwardDataRequest(rq)
    }
  }

  if packet.DataReply != nil {
    rp := packet.DataReply
    if rp.Destination == gossiper.Name {
      gossiper.ProcessDataReply(rp)
    } else {
      rp.HopLimit--
      gossiper.ForwardDataReply(rp)
    }
  }
}

func handleClientMessage(gossiper *Gossiper, client *Client, msg *Message) {
  if msg.File != "" && msg.Request != "" {
    requested, err := hex.DecodeString(msg.Request)
    utils.CheckError(err)
    gossiper.AddStubFile(msg.Request, requested, msg.File)
    gossiper.SendDataRequest(&DataRequest{
      Origin: gossiper.Name,
      Destination: msg.Destination,
      HopLimit: 10,
      HashValue: requested,
    })
  } else if msg.File != "" {
    file, err := os.Open("_SharedFiles/" + msg.File)
    utils.CheckError(err)
    fileStat, err := file.Stat()
    utils.CheckError(err)
    end := fileStat.Size()

    numChunks := end / FILE_CHUNK_SIZE

    chunks := make(map[string][]byte)
    metaFile := make([]byte, 0, 32 * numChunks)
    offset := int64(0)
    for offset < end {
      readLength := utils.Min(FILE_CHUNK_SIZE, end - offset)
      chunk := make([]byte, readLength)
      count, err := file.ReadAt(chunk, offset)
      utils.CheckError(err)
      chunkHash := sha256.Sum256(chunk)
      chunks[hex.EncodeToString(chunkHash[:])] = chunk
      metaFile = append(metaFile, chunkHash[:]...)
      offset += int64(count)
    }
    metaHash := sha256.Sum256(metaFile)
    gossiper.AddFile(msg.File, end, metaHash, metaFile, chunks, numChunks)
  }

  if msg.Destination != "" && msg.Text != "" {
    privateMessage := &PrivateMessage{
      Origin: gossiper.Name,
      ID: 0,
      Text: msg.Text,
      Destination: msg.Destination,
      HopLimit: 10,
    }
    gossiper.RecordPrivate(privateMessage)
    gossiper.ForwardPrivate(privateMessage)
  } else if *simpleMode && msg.Text != "" {
    packet := &GossipPacket{
      &SimpleMessage{
        gossiper.Name,
        *gossipAddr,
        msg.Text,
      },
      nil,
      nil,
      nil,
      nil,
      nil,
    }
    gossiper.ForwardToAllPeers(gossiper.Address, packet)
  } else if msg.Text != "" {
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
