function $(sel) {
  return document.querySelector(sel);
}

document.addEventListener("DOMContentLoaded", async () => {
  $("#node-id").textContent = await getNodeId();

  updateStatus();
  setInterval(updateStatus, 3000);

  $("#send-form").addEventListener("submit", e => {
    e.preventDefault();
    sendMessage($("#send-input").value);
    $("#send-input").value = "";
  });

  $("#add-peer-form").addEventListener("submit", e => {
    e.preventDefault();
    addPeer($("#add-peer-input").value);
    $("#add-peer-input").value = "";
  });
});

let receivedMessages = 0;

async function updateStatus() {
  const peers = await getAllPeers();
  $("#node-peers").textContent = "";
  $("#node-peers").append(...peers.map((p) => {
    const li = document.createElement("li");
    li.textContent = p;
    return li;
  }));

  const messages = await getAllMessages();
  $("#messages").append(...messages.slice(receivedMessages).map(({ Origin, ID, Text, Destination }) => {
    const li = document.createElement("li");
    li.classList.toggle("private", !!Destination);

    const originEl = document.createElement("span");
    originEl.textContent = Origin;
    originEl.className = "message-origin";

    const idEl = document.createElement("span");
    if (!Destination) {
      idEl.textContent = ID;
    }
    idEl.className = "message-id";

    const contentsEl = document.createElement("p");
    contentsEl.textContent = Text;
    contentsEl.className = "message-contents";

    li.append(originEl, idEl, contentsEl);
    return li;
  }));
  receivedMessages = messages.length;
}

async function getNodeId() {
  const response = await fetch("/id");
  return await response.text();
}

async function getAllPeers() {
  const response = await fetch("/node");
  return JSON.parse(await response.text());
}

async function getAllMessages() {
  const response = await fetch("/message");
  return JSON.parse(await response.text());
}

function sendMessage(message) {
  const headers = new Headers();
  headers.append("Content-Type", "application/json");

  return fetch("/message", {
    method: "POST",
    headers,
    body: JSON.stringify({
      Text: message,
      Destination: "",
    }),
  });
}

function addPeer(peerAddr) {
  const headers = new Headers();
  headers.append("Content-Type", "text/plain");

  return fetch("/node", {
    method: "POST",
    headers,
    body: peerAddr,
  });
}
