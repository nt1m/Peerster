package utils

import (
  "log"
  "time"
)

func CheckError(err error) {
  if err != nil {
    log.Fatal("ERROR:", err)
  }
}

func SetTimeout(callback func(), duration time.Duration) chan bool {
  stop := make(chan bool)
  go func() {
    ticker := time.NewTicker(duration)
    defer ticker.Stop()
    select {
    case <-ticker.C:
      callback()
    case <-stop:
    }
  }()
  return stop
}

func Assert(condition bool) {
  if !condition {
    panic("Assert failed")
  }
}

func Min(a, b int64) int64 {
  if a < b {
    return a
  }
  return b
}
