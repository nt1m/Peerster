package types

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
