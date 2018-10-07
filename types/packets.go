package types

import (
  "fmt"
  "github.com/dedis/protobuf"
  "Peerster/utils"
)

type SimpleMessage struct {
  OriginalName string
  RelayPeerAddr string
  Contents string
}

type RumorMessage struct {
  Origin string
  ID uint32
  Text string
}

type StatusPacket struct {
  Want []PeerStatus
}

type PeerStatus struct {
  Identifier string
  NextID uint32
}

type Message struct {
  Text string
}

type GossipPacket struct {
  Simple *SimpleMessage
  Rumor *RumorMessage
  Status *StatusPacket
}

func (packet* StatusPacket) ToMap() map[string]uint32 {
  statusMap := make(map[string]uint32)
  for _, status := range packet.Want {
    statusMap[status.Identifier] = status.NextID
  }
  return statusMap
}

func EncodePacket(packet *GossipPacket) []byte {
  packetBytes, err := protobuf.Encode(packet)
  utils.CheckError(err)
  return packetBytes
}

func (msg *SimpleMessage) Log() {
  fmt.Println("SIMPLE MESSAGE origin", msg.OriginalName, "from", msg.RelayPeerAddr, "contents", msg.Contents)
}

func (msg *RumorMessage) Log(relayAddress string) {
  fmt.Println("RUMOR origin", msg.Origin, "from", relayAddress, "ID", msg.ID, "contents", msg.Text)
}

func (packet *StatusPacket) Log(relayAddress string) {
  str := ""
  for i, status := range packet.Want {
    if i > 0 {
      str += " "
    }
    str += "peer " + status.Identifier + " nextID " + string(status.NextID)
  }
  fmt.Println("STATUS from", relayAddress, "peer", str)
}
